package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/pkg/evidence"
	"samebits.com/evidra-benchmark/pkg/version"
	promptdata "samebits.com/evidra-benchmark/prompts"
)

// Options configures the benchmark MCP server.
type Options struct {
	Name             string
	Version          string
	EvidencePath     string
	Environment      string
	RetryTracker     bool
	BestEffortWrites bool
	Signer           evidence.Signer // required: signs evidence entries
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
	PrescriptionID string                 `json:"prescription_id"`
	ExitCode       int                    `json:"exit_code"`
	ArtifactDigest string                 `json:"artifact_digest,omitempty"`
	Actor          InputActor             `json:"actor"`
	ExternalRefs   []evidence.ExternalRef `json:"external_refs,omitempty"`
	SessionID      string                 `json:"session_id,omitempty"`
	OperationID    string                 `json:"operation_id,omitempty"`
	SpanID         string                 `json:"span_id,omitempty"`
	ParentSpanID   string                 `json:"parent_span_id,omitempty"`
}

// ReportOutput is returned by the report tool.
type ReportOutput struct {
	OK       bool     `json:"ok"`
	ReportID string   `json:"report_id"`
	Signals  []string `json:"signals,omitempty"`
	Error    *ErrInfo `json:"error,omitempty"`
}

// ErrInfo represents an error in tool output.
type ErrInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type prescribeHandler struct {
	service *BenchmarkService
}

type reportHandler struct {
	service *BenchmarkService
}

// BenchmarkService provides prescribe and report operations.
type BenchmarkService struct {
	evidencePath     string
	retryTracker     *RetryTracker
	signer           evidence.Signer
	bestEffortWrites bool
	lifecycle        *lifecycle.Service
}

const (
	defaultPrescribeToolDescription = "Analyze an infrastructure artifact BEFORE execution. " +
		"Returns risk level, canonical digests, and a prescription ID. " +
		"Call this BEFORE running kubectl apply, terraform apply, or similar commands."

	defaultReportToolDescription = "Report the outcome of an infrastructure operation AFTER execution. " +
		"Provide the prescription_id from a previous prescribe call and the exit code."

	defaultGetEventToolDescription = "Look up an evidence record by event_id."

	defaultInitializeInstructions = "Evidra Benchmark — flight recorder for infrastructure automation. " +
		"Call `prescribe` BEFORE any infrastructure operation and `report` AFTER."
)

var (
	prescribeToolDescription = defaultPrescribeToolDescription
	reportToolDescription    = defaultReportToolDescription
	getEventToolDescription  = defaultGetEventToolDescription
	initializeInstructions   = defaultInitializeInstructions
	contractVersion          = "v1.0.1"
	contractSkillVersion     = "1.0.1"
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

// NewServer creates a new benchmark MCP server with prescribe and report tools.
func NewServer(opts Options) (*mcp.Server, error) {
	if opts.Name == "" {
		opts.Name = "evidra-benchmark"
	}
	opts.Version = defaultServerVersion(opts.Version)

	svc := &BenchmarkService{
		evidencePath:     opts.EvidencePath,
		signer:           opts.Signer,
		bestEffortWrites: opts.BestEffortWrites,
	}
	if opts.RetryTracker {
		svc.retryTracker = NewRetryTracker(10 * time.Minute)
	}
	svc.lifecycle = lifecycle.NewService(lifecycle.Options{
		EvidencePath:     svc.evidencePath,
		Signer:           svc.signer,
		RetryTracker:     toRetryRecorder(svc.retryTracker),
		BestEffortWrites: svc.bestEffortWrites,
	})

	prescribe := &prescribeHandler{service: svc}
	report := &reportHandler{service: svc}
	getEvent := &getEventHandler{service: svc}

	prescribeSchema, err := loadInputSchema(prescribeSchemaBytes, "schemas/prescribe.schema.json")
	if err != nil {
		return nil, err
	}
	reportSchema, err := loadInputSchema(reportSchemaBytes, "schemas/report.schema.json")
	if err != nil {
		return nil, err
	}
	getEventSchema, err := loadInputSchema(getEventSchemaBytes, "schemas/get_event.schema.json")
	if err != nil {
		return nil, err
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
			ReadOnlyHint:    true,
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
		InputSchema: getEventSchema,
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

	return server, nil
}

func defaultServerVersion(input string) string {
	v := strings.TrimSpace(input)
	if v != "" {
		return v
	}
	return version.Version
}

func (h *prescribeHandler) Handle(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input PrescribeInput,
) (*mcp.CallToolResult, PrescribeOutput, error) {
	output := h.service.Prescribe(input)
	return &mcp.CallToolResult{}, output, nil
}

func (h *reportHandler) Handle(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input ReportInput,
) (*mcp.CallToolResult, ReportOutput, error) {
	output := h.service.Report(input)
	return &mcp.CallToolResult{}, output, nil
}

func (s *BenchmarkService) lifecycleService() *lifecycle.Service {
	if s.lifecycle == nil {
		s.lifecycle = lifecycle.NewService(lifecycle.Options{
			EvidencePath:     s.evidencePath,
			Signer:           s.signer,
			RetryTracker:     toRetryRecorder(s.retryTracker),
			BestEffortWrites: s.bestEffortWrites,
		})
	}
	return s.lifecycle
}

// Prescribe records intent and returns risk assessment metadata.
func (s *BenchmarkService) Prescribe(input PrescribeInput) PrescribeOutput {
	out, err := s.lifecycleService().Prescribe(context.Background(), toLifecyclePrescribeInput(input))
	if err != nil {
		return PrescribeOutput{
			OK:    false,
			Error: lifecycleErrInfo(err),
		}
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

// Report records the outcome of an operation, matching it to a prescription.
func (s *BenchmarkService) Report(input ReportInput) ReportOutput {
	out, err := s.lifecycleService().Report(context.Background(), toLifecycleReportInput(input))
	if err != nil {
		return ReportOutput{
			OK:    false,
			Error: lifecycleErrInfo(err),
		}
	}
	return ReportOutput{
		OK:       true,
		ReportID: out.ReportID,
	}
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
	service *BenchmarkService
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

func (s *BenchmarkService) GetEvent(eventID string) GetEventOutput {
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

func (s *BenchmarkService) readResourceEvent(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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

func (s *BenchmarkService) readResourceManifest(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
