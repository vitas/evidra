package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra/internal/assessment"
	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/lifecycle"
	"samebits.com/evidra/internal/score"
	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
	promptdata "samebits.com/evidra/prompts"
)

// ForwardFunc is an optional callback to forward evidence entries to the API.
type ForwardFunc func(ctx context.Context, entry json.RawMessage)

// Options configures the benchmark MCP server.
type Options struct {
	Name               string
	Version            string
	EvidencePath       string
	Environment        string
	RetryTracker       bool
	BestEffortWrites   bool
	ScoringProfilePath string
	Signer             evidence.Signer // required: signs evidence entries
	Forward            ForwardFunc     // optional: best-effort forward to API
}

// InputActor identifies the caller in a prescribe request.
type InputActor struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	Origin       string `json:"origin"`
	InstanceID   string `json:"instance_id,omitempty"`
	Version      string `json:"version,omitempty"`
	SkillVersion string `json:"skill_version,omitempty"`
}

// PrescribeInput is the input schema for the prescribe tool.
type PrescribeInput struct {
	Actor           InputActor             `json:"actor"`
	Tool            string                 `json:"tool"`
	Operation       string                 `json:"operation"`
	RawArtifact     string                 `json:"raw_artifact"`
	Environment     string                 `json:"environment,omitempty"`
	CanonicalAction *canon.CanonicalAction `json:"canonical_action,omitempty"`
	SessionID       string                 `json:"session_id,omitempty"`
	OperationID     string                 `json:"operation_id,omitempty"`
	Attempt         int                    `json:"attempt,omitempty"`
	TraceID         string                 `json:"trace_id,omitempty"`
	SpanID          string                 `json:"span_id,omitempty"`
	ParentSpanID    string                 `json:"parent_span_id,omitempty"`
	ScopeDimensions map[string]string      `json:"scope_dimensions,omitempty"`
}

// PrescribeOutput is returned by the prescribe tool.
type PrescribeOutput struct {
	OK             bool     `json:"ok"`
	PrescriptionID string   `json:"prescription_id"`
	RiskLevel      string   `json:"risk_level"`
	RiskTags       []string `json:"risk_tags,omitempty"`
	ArtifactDigest string   `json:"artifact_digest"`
	IntentDigest   string   `json:"intent_digest"`
	ShapeHash      string   `json:"resource_shape_hash"`
	ResourceCount  int      `json:"resource_count"`
	OperationClass string   `json:"operation_class"`
	ScopeClass     string   `json:"scope_class"`
	CanonVersion   string   `json:"canon_version"`
	RetryCount     int      `json:"retry_count,omitempty"`
	Error          *ErrInfo `json:"error,omitempty"`
}

// ReportInput is the input schema for the report tool.
type ReportInput struct {
	PrescriptionID  string                    `json:"prescription_id"`
	Verdict         evidence.Verdict          `json:"verdict"`
	ExitCode        *int                      `json:"exit_code,omitempty"`
	DecisionContext *evidence.DecisionContext `json:"decision_context,omitempty"`
	ArtifactDigest  string                    `json:"artifact_digest,omitempty"`
	Actor           InputActor                `json:"actor"`
	ExternalRefs    []evidence.ExternalRef    `json:"external_refs,omitempty"`
	SessionID       string                    `json:"session_id,omitempty"`
	OperationID     string                    `json:"operation_id,omitempty"`
	SpanID          string                    `json:"span_id,omitempty"`
	ParentSpanID    string                    `json:"parent_span_id,omitempty"`
}

// ReportOutput is returned by the report tool.
type ReportOutput struct {
	OK               bool                      `json:"ok"`
	ReportID         string                    `json:"report_id"`
	PrescriptionID   string                    `json:"prescription_id"`
	ExitCode         *int                      `json:"exit_code,omitempty"`
	Verdict          evidence.Verdict          `json:"verdict"`
	DecisionContext  *evidence.DecisionContext `json:"decision_context,omitempty"`
	Score            float64                   `json:"score"`
	ScoreBand        string                    `json:"score_band"`
	ScoringProfileID string                    `json:"scoring_profile_id"`
	SignalSummary    map[string]int            `json:"signal_summary"`
	Basis            assessment.Basis          `json:"basis"`
	Confidence       score.Confidence          `json:"confidence"`
	Error            *ErrInfo                  `json:"error,omitempty"`
}

// ErrInfo represents an error in tool output.
type ErrInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type prescribeHandler struct {
	service *MCPService
}

type reportHandler struct {
	service *MCPService
}

// MCPService provides prescribe and report operations.
type MCPService struct {
	evidencePath      string
	retryTracker      *RetryTracker
	signer            evidence.Signer
	bestEffortWrites  bool
	lifecycle         *lifecycle.Service
	forwardFunc       ForwardFunc
	scoringProfile    score.Profile
	assessmentTracker *assessment.Tracker
	initOnce          sync.Once
	initErr           error
	closeOnce         sync.Once
	closeErr          error
}

const (
	defaultPrescribeToolDescription = "Analyze an infrastructure artifact BEFORE execution. " +
		"Returns risk level, canonical digests, and a prescription ID. " +
		"Call this BEFORE running kubectl apply, terraform apply, or similar commands."

	defaultReportToolDescription = "Report the terminal outcome of an infrastructure operation or decision. " +
		"Provide the prescription_id from a previous prescribe call, an explicit verdict, and for declined decisions a short operational reason."

	defaultGetEventToolDescription = "Look up an evidence record by event_id."

	defaultInitializeInstructions = "Evidra — Flight recorder for AI infrastructure agents. " +
		"Call `prescribe` BEFORE any infrastructure operation and `report` with an explicit verdict AFTER execution or decision."
)

var (
	prescribeToolDescription = defaultPrescribeToolDescription
	reportToolDescription    = defaultReportToolDescription
	getEventToolDescription  = defaultGetEventToolDescription
	initializeInstructions   = defaultInitializeInstructions
	contractVersion          = promptdata.DefaultContractVersion
	contractSkillVersion     = promptdata.DefaultContractSkillVersion
)

func init() {
	if s, err := promptdata.Read(promptdata.MCPPrescribeDescriptionPath); err == nil {
		prescribeToolDescription = promptdata.StripContractHeader(s)
	}
	if s, err := promptdata.Read(promptdata.MCPReportDescriptionPath); err == nil {
		reportToolDescription = promptdata.StripContractHeader(s)
	}
	if s, err := promptdata.Read(promptdata.MCPGetEventDescriptionPath); err == nil {
		getEventToolDescription = promptdata.StripContractHeader(s)
	}
	if instructions, cv, sv, err := promptdata.ReadMCPInitializeInstructions(); err == nil {
		initializeInstructions = instructions
		contractVersion = cv
		contractSkillVersion = sv
	}
}

// NewServer creates a new Evidra MCP server with prescribe and report tools.
func NewServer(opts Options) (*mcp.Server, error) {
	server, _, err := NewServerWithCleanup(opts)
	return server, err
}

// NewServerWithCleanup creates a new Evidra MCP server and returns a cleanup function.
func NewServerWithCleanup(opts Options) (*mcp.Server, func() error, error) {
	if opts.Name == "" {
		opts.Name = "evidra-benchmark"
	}
	opts.Version = defaultServerVersion(opts.Version)

	svc, err := newMCPService(opts)
	if err != nil {
		return nil, nil, err
	}

	prescribe := &prescribeHandler{service: svc}
	report := &reportHandler{service: svc}
	getEvent := &getEventHandler{service: svc}

	prescribeSchema, err := loadSchema(prescribeSchemaBytes, "schemas/prescribe.schema.json")
	if err != nil {
		return nil, nil, err
	}
	reportSchema, err := loadSchema(reportSchemaBytes, "schemas/report.schema.json")
	if err != nil {
		return nil, nil, err
	}
	getEventSchema, err := loadSchema(getEventSchemaBytes, "schemas/get_event.schema.json")
	if err != nil {
		return nil, nil, err
	}
	getEventOutputSchema, err := loadSchema(getEventOutputSchemaBytes, "schemas/get_event.output.schema.json")
	if err != nil {
		return nil, nil, err
	}

	server := mcp.NewServer(
		&mcp.Implementation{Name: opts.Name, Version: opts.Version},
		&mcp.ServerOptions{
			Instructions: initializeInstructions,
		},
	)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "prescribe",
		Title:       "Record Infrastructure Intent",
		Description: prescribeToolDescription,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Prescribe",
			ReadOnlyHint:    false,
			IdempotentHint:  false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
		InputSchema: prescribeSchema,
	}, prescribe.Handle)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "report",
		Title:       "Report Operation Result",
		Description: reportToolDescription,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Report",
			ReadOnlyHint:    false,
			IdempotentHint:  false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
		InputSchema: reportSchema,
	}, report.Handle)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_event",
		Title:       "Get Evidence Event",
		Description: getEventToolDescription,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Evidence Lookup",
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
		InputSchema:  getEventSchema,
		OutputSchema: getEventOutputSchema,
	}, getEvent.Handle)

	// Evidence resources
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "evidra-event",
		Title:       "Evidence Event Record",
		Description: "Read a specific evidence record by event_id.",
		MIMEType:    "application/json",
		URITemplate: "evidra://event/{event_id}",
	}, svc.readResourceEvent)
	server.AddResource(&mcp.Resource{
		Name:        "evidra-evidence-manifest",
		Title:       "Evidence Manifest",
		Description: "Read evidence manifest for segmented store.",
		MIMEType:    "application/json",
		URI:         "evidra://evidence/manifest",
	}, svc.readResourceManifest)

	return server, svc.Close, nil
}

func newMCPService(opts Options) (*MCPService, error) {
	svc := &MCPService{
		evidencePath:      opts.EvidencePath,
		signer:            opts.Signer,
		bestEffortWrites:  opts.BestEffortWrites,
		forwardFunc:       opts.Forward,
		assessmentTracker: assessment.NewTracker(opts.EvidencePath),
	}
	if opts.RetryTracker {
		svc.retryTracker = NewRetryTracker(10 * time.Minute)
	}
	profile, err := score.ResolveProfile(opts.ScoringProfilePath)
	if err != nil {
		return nil, err
	}
	svc.scoringProfile = profile
	svc.lifecycle = svc.newLifecycleService()
	return svc, nil
}

func defaultServerVersion(input string) string {
	v := strings.TrimSpace(input)
	if v != "" {
		return v
	}
	return version.Version
}

func (h *prescribeHandler) Handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input PrescribeInput,
) (*mcp.CallToolResult, PrescribeOutput, error) {
	output := h.service.PrescribeCtx(ctx, input)
	return &mcp.CallToolResult{}, output, nil
}

func (h *reportHandler) Handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input ReportInput,
) (*mcp.CallToolResult, ReportOutput, error) {
	output := h.service.ReportCtx(ctx, input)
	return &mcp.CallToolResult{}, output, nil
}

func (s *MCPService) newLifecycleService() *lifecycle.Service {
	return lifecycle.NewService(lifecycle.Options{
		EvidencePath:     s.evidencePath,
		Signer:           s.signer,
		RetryTracker:     toRetryRecorder(s.retryTracker),
		BestEffortWrites: s.bestEffortWrites,
	})
}

func (s *MCPService) ensureInitialized() error {
	s.initOnce.Do(func() {
		if s.scoringProfile.ID == "" {
			s.scoringProfile, s.initErr = score.ResolveProfile("")
			if s.initErr != nil {
				return
			}
		}
		if s.lifecycle == nil {
			s.lifecycle = s.newLifecycleService()
		}
	})
	return s.initErr
}

func (s *MCPService) lifecycleService() (*lifecycle.Service, error) {
	if err := s.ensureInitialized(); err != nil {
		return nil, err
	}
	return s.lifecycle, nil
}

func (s *MCPService) Close() error {
	s.closeOnce.Do(func() {
		if s.retryTracker != nil {
			s.retryTracker.Stop()
		}
	})
	return s.closeErr
}

// PrescribeCtx records intent and returns risk assessment metadata with context propagation.
func (s *MCPService) PrescribeCtx(ctx context.Context, input PrescribeInput) PrescribeOutput {
	svc, err := s.lifecycleService()
	if err != nil {
		return PrescribeOutput{
			OK:    false,
			Error: &ErrInfo{Code: string(lifecycle.ErrCodeInternal), Message: err.Error()},
		}
	}

	out, err := svc.Prescribe(ctx, toLifecyclePrescribeInput(input))
	if err != nil {
		return PrescribeOutput{
			OK:    false,
			Error: lifecycleErrInfo(err),
		}
	}

	if out.Persisted {
		s.observeWrittenEntry(out.Entry)
		s.tryForwardEntry(ctx, out.RawEntry)
	}

	return PrescribeOutput{
		OK:             true,
		PrescriptionID: out.PrescriptionID,
		RiskLevel:      out.RiskLevel,
		RiskTags:       out.RiskTags,
		ArtifactDigest: out.ArtifactDigest,
		IntentDigest:   out.IntentDigest,
		ShapeHash:      out.ShapeHash,
		ResourceCount:  out.ResourceCount,
		OperationClass: out.OperationClass,
		ScopeClass:     out.ScopeClass,
		CanonVersion:   out.CanonVersion,
		RetryCount:     out.RetryCount,
	}
}

// ReportCtx records the outcome of an operation with context propagation.
func (s *MCPService) ReportCtx(ctx context.Context, input ReportInput) ReportOutput {
	svc, err := s.lifecycleService()
	if err != nil {
		return ReportOutput{
			OK:              false,
			PrescriptionID:  input.PrescriptionID,
			ExitCode:        input.ExitCode,
			Verdict:         input.Verdict,
			DecisionContext: input.DecisionContext,
			SignalSummary:   map[string]int{},
			Error:           &ErrInfo{Code: string(lifecycle.ErrCodeInternal), Message: err.Error()},
		}
	}

	out, err := svc.Report(ctx, toLifecycleReportInput(input))
	if err != nil {
		return ReportOutput{
			OK:              false,
			PrescriptionID:  input.PrescriptionID,
			ExitCode:        input.ExitCode,
			Verdict:         input.Verdict,
			DecisionContext: input.DecisionContext,
			SignalSummary:   map[string]int{},
			Error:           lifecycleErrInfo(err),
		}
	}

	if out.Persisted {
		s.observeWrittenEntry(out.Entry)
		s.tryForwardEntry(ctx, out.RawEntry)
	}

	snapshot, err := s.sessionSnapshot(out.SessionID)
	if err != nil {
		return ReportOutput{
			OK:              false,
			PrescriptionID:  out.PrescriptionID,
			ExitCode:        out.ExitCode,
			Verdict:         out.Verdict,
			DecisionContext: out.DecisionContext,
			SignalSummary:   map[string]int{},
			Error:           &ErrInfo{Code: string(lifecycle.ErrCodeInternal), Message: "failed to build assessment snapshot"},
		}
	}

	return ReportOutput{
		OK:               true,
		ReportID:         out.ReportID,
		PrescriptionID:   out.PrescriptionID,
		ExitCode:         out.ExitCode,
		Verdict:          out.Verdict,
		DecisionContext:  out.DecisionContext,
		Score:            snapshot.Score,
		ScoreBand:        snapshot.ScoreBand,
		ScoringProfileID: snapshot.ScoringProfileID,
		SignalSummary:    snapshot.SignalSummary,
		Basis:            snapshot.Basis,
		Confidence:       snapshot.Confidence,
	}
}

// Prescribe is a convenience wrapper that calls PrescribeCtx with context.Background().
func (s *MCPService) Prescribe(input PrescribeInput) PrescribeOutput {
	return s.PrescribeCtx(context.Background(), input)
}

// Report is a convenience wrapper that calls ReportCtx with context.Background().
func (s *MCPService) Report(input ReportInput) ReportOutput {
	return s.ReportCtx(context.Background(), input)
}

func (s *MCPService) observeWrittenEntry(entry evidence.EvidenceEntry) {
	if s.assessmentTracker == nil || entry.EntryID == "" {
		return
	}
	_ = s.assessmentTracker.Observe(entry)
}

func (s *MCPService) sessionSnapshot(sessionID string) (assessment.Snapshot, error) {
	if s.assessmentTracker != nil {
		return s.assessmentTracker.Snapshot(sessionID, s.scoringProfile)
	}
	return assessment.BuildAtPathWithProfile(s.evidencePath, sessionID, s.scoringProfile)
}

// tryForwardEntry best-effort forwards a freshly written entry.
func (s *MCPService) tryForwardEntry(ctx context.Context, entry json.RawMessage) {
	if s.forwardFunc == nil || len(entry) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.forwardFunc(ctx, entry)
}

func lifecycleErrInfo(err error) *ErrInfo {
	code := lifecycle.ErrorCode(err)
	if code == "" {
		code = lifecycle.ErrCodeInternal
	}
	return &ErrInfo{Code: string(code), Message: err.Error()}
}

// --- get_event tool ---

type getEventHandler struct {
	service *MCPService
}

type getEventInput struct {
	EventID string `json:"event_id"`
}

// GetEventOutput is returned by the get_event tool.
type GetEventOutput struct {
	OK    bool                    `json:"ok"`
	Entry *evidence.EvidenceEntry `json:"entry,omitempty"`
	Error *ErrInfo                `json:"error,omitempty"`
}

func (h *getEventHandler) Handle(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input getEventInput,
) (*mcp.CallToolResult, GetEventOutput, error) {
	output := h.service.GetEvent(input.EventID)
	return &mcp.CallToolResult{}, output, nil
}

func (s *MCPService) GetEvent(eventID string) GetEventOutput {
	if eventID == "" {
		return GetEventOutput{OK: false, Error: &ErrInfo{Code: "invalid_input", Message: "event_id is required"}}
	}
	if s.evidencePath == "" {
		return GetEventOutput{OK: false, Error: &ErrInfo{Code: "no_evidence_path", Message: "evidence path not configured"}}
	}
	entry, found, err := evidence.FindEntryByID(s.evidencePath, eventID)
	if err != nil {
		return GetEventOutput{OK: false, Error: &ErrInfo{Code: "internal_error", Message: "failed to read evidence"}}
	}
	if !found {
		return GetEventOutput{OK: false, Error: &ErrInfo{Code: "not_found", Message: "event_id not found"}}
	}
	return GetEventOutput{OK: true, Entry: &entry}
}

// --- resource handlers ---

func (s *MCPService) readResourceEvent(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	eventID := strings.TrimPrefix(req.Params.URI, "evidra://event/")
	if eventID == "" || eventID == req.Params.URI {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	if s.evidencePath == "" {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	entry, found, err := evidence.FindEntryByID(s.evidencePath, eventID)
	if err != nil || !found {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	b, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: string(b)}}}, nil
}

func (s *MCPService) readResourceManifest(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if s.evidencePath == "" {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	m, err := evidence.LoadManifest(s.evidencePath)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "evidra://evidence/manifest", MIMEType: "application/json", Text: string(b)}}}, nil
}

func boolPtr(v bool) *bool {
	return &v
}
