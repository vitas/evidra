package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oklog/ulid/v2"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/risk"
	"samebits.com/evidra-benchmark/pkg/evidence"
	"samebits.com/evidra-benchmark/pkg/version"
)

// Options configures the benchmark MCP server.
type Options struct {
	Name         string
	Version      string
	EvidencePath string
	Environment  string
	RetryTracker bool
	Signer       evidence.Signer // required: signs evidence entries
}

// InputActor identifies the caller in a prescribe request.
type InputActor struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Origin     string `json:"origin"`
	InstanceID string `json:"instance_id,omitempty"`
	Version    string `json:"version,omitempty"`
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
	evidencePath string
	retryTracker *RetryTracker
	signer       evidence.Signer
	lastTraceID  string
	lastActor    evidence.Actor
}

const (
	prescribeToolDescription = "Analyze an infrastructure artifact BEFORE execution. " +
		"Returns risk level, canonical digests, and a prescription ID. " +
		"Call this BEFORE running kubectl apply, terraform apply, or similar commands."

	reportToolDescription = "Report the outcome of an infrastructure operation AFTER execution. " +
		"Provide the prescription_id from a previous prescribe call and the exit code."

	getEventToolDescription = "Look up an evidence record by event_id."

	initializeInstructions = "Evidra Benchmark — flight recorder for infrastructure automation. " +
		"Call `prescribe` BEFORE any infrastructure operation and `report` AFTER."
)

// NewServer creates a new benchmark MCP server with prescribe and report tools.
func NewServer(opts Options) (*mcp.Server, error) {
	if opts.Name == "" {
		opts.Name = "evidra-benchmark"
	}
	if opts.Version == "" {
		opts.Version = "v0.3.0-dev"
	}

	svc := &BenchmarkService{
		evidencePath: opts.EvidencePath,
		signer:       opts.Signer,
	}
	if opts.RetryTracker {
		svc.retryTracker = NewRetryTracker(10 * time.Minute)
	}

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

// Prescribe canonicalizes the artifact, runs risk detectors, and records
// a prescription in the evidence chain.
func (s *BenchmarkService) Prescribe(input PrescribeInput) PrescribeOutput {
	rawArtifact := []byte(input.RawArtifact)

	var cr canon.CanonResult

	if input.CanonicalAction != nil {
		// Pre-canonicalized path: caller already computed the canonical action.
		actionJSON, _ := json.Marshal(input.CanonicalAction)
		cr = canon.CanonResult{
			ArtifactDigest:  canon.SHA256Hex(rawArtifact),
			IntentDigest:    canon.ComputeIntentDigest(*input.CanonicalAction),
			CanonicalAction: *input.CanonicalAction,
			CanonVersion:    "external/v1",
			RawAction:       actionJSON,
		}
	} else {
		// Standard path: run adapter.
		cr = canon.Canonicalize(input.Tool, input.Operation, input.Environment, rawArtifact)
		if cr.ParseError != nil {
			// Write canonicalization_failure evidence entry
			if s.evidencePath != "" {
				failPayload, _ := json.Marshal(evidence.CanonFailurePayload{
					ErrorCode:    "parse_error",
					ErrorMessage: cr.ParseError.Error(),
					Adapter:      cr.CanonVersion,
					RawDigest:    cr.ArtifactDigest,
				})
				actor := evidence.Actor{
					Type:       input.Actor.Type,
					ID:         input.Actor.ID,
					Provenance: input.Actor.Origin,
				}
				lastHash, _ := evidence.LastHashAtPath(s.evidencePath)
				entry, buildErr := evidence.BuildEntry(evidence.EntryBuildParams{
					Type:           evidence.EntryTypeCanonFailure,
					TraceID:        evidence.GenerateTraceID(),
					Actor:          actor,
					ArtifactDigest: cr.ArtifactDigest,
					Payload:        failPayload,
					PreviousHash:   lastHash,
					SpecVersion:    "0.3.0",
					AdapterVersion: version.Version,
					Signer:         s.signer,
				})
				if buildErr == nil {
					_ = evidence.AppendEntryAtPath(s.evidencePath, entry) // best-effort for failure entries
				}
			}
			return PrescribeOutput{
				OK:    false,
				Error: &ErrInfo{Code: "parse_error", Message: cr.ParseError.Error()},
			}
		}
	}

	// Run risk detectors on raw artifact regardless of canonicalization path
	riskTags := risk.RunAll(cr.CanonicalAction, rawArtifact)
	riskLevel := risk.ElevateRiskLevel(
		risk.RiskLevel(cr.CanonicalAction.OperationClass, cr.CanonicalAction.ScopeClass),
		riskTags,
	)

	// Track retries
	var retryCount int
	if s.retryTracker != nil {
		retryCount = s.retryTracker.Record(cr.IntentDigest, cr.CanonicalAction.ResourceShapeHash)
	}

	// Determine canon_source
	canonSource := "adapter"
	if input.CanonicalAction != nil {
		canonSource = "external"
	}

	// Build prescription payload
	prescPayload := evidence.PrescriptionPayload{
		CanonicalAction: cr.RawAction,
		RiskLevel:       riskLevel,
		RiskTags:        riskTags,
		TTLMs:           evidence.DefaultTTLMs,
		CanonSource:     canonSource,
	}
	payloadJSON, err := json.Marshal(prescPayload)
	if err != nil {
		return PrescribeOutput{
			OK:    false,
			Error: &ErrInfo{Code: "internal_error", Message: "failed to marshal prescription payload"},
		}
	}

	// Map invocation.Actor to evidence.Actor (Origin -> Provenance)
	actor := evidence.Actor{
		Type:       input.Actor.Type,
		ID:         input.Actor.ID,
		Provenance: input.Actor.Origin,
		InstanceID: input.Actor.InstanceID,
		Version:    input.Actor.Version,
	}

	// Use caller-provided trace ID or generate one per operation
	traceID := input.TraceID
	if traceID == "" {
		traceID = evidence.GenerateTraceID()
	}

	// Get last hash for chain
	lastHash, _ := evidence.LastHashAtPath(s.evidencePath)

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:            evidence.EntryTypePrescribe,
		SessionID:       input.SessionID,
		TraceID:         traceID,
		SpanID:          input.SpanID,
		ParentSpanID:    input.ParentSpanID,
		Actor:           actor,
		IntentDigest:    cr.IntentDigest,
		ArtifactDigest:  cr.ArtifactDigest,
		Payload:         payloadJSON,
		PreviousHash:    lastHash,
		ScopeDimensions: input.ScopeDimensions,
		SpecVersion:     "0.3.0",
		CanonVersion:    cr.CanonVersion,
		AdapterVersion:  version.Version,
		Signer:          s.signer,
	})
	if err != nil {
		return PrescribeOutput{
			OK:    false,
			Error: &ErrInfo{Code: "internal_error", Message: err.Error()},
		}
	}

	// Set prescription_id = entry_id for consistent identity
	prescPayload.PrescriptionID = entry.EntryID
	payloadJSON, _ = json.Marshal(prescPayload)
	entry.Payload = payloadJSON

	// Recompute hash and signature after payload mutation.
	if err := evidence.RehashEntry(&entry, s.signer); err != nil {
		return PrescribeOutput{
			OK:    false,
			Error: &ErrInfo{Code: "internal_error", Message: err.Error()},
		}
	}

	// Store actor and trace ID for subsequent report calls
	s.lastActor = actor
	s.lastTraceID = traceID

	// Write to evidence store
	if s.evidencePath != "" {
		if err := evidence.AppendEntryAtPath(s.evidencePath, entry); err != nil {
			return PrescribeOutput{
				OK:    false,
				Error: &ErrInfo{Code: "evidence_write_failed", Message: err.Error()},
			}
		}
	}

	return PrescribeOutput{
		OK:             true,
		PrescriptionID: entry.EntryID,
		RiskLevel:      riskLevel,
		RiskTags:       riskTags,
		ArtifactDigest: cr.ArtifactDigest,
		IntentDigest:   cr.IntentDigest,
		ShapeHash:      cr.CanonicalAction.ResourceShapeHash,
		ResourceCount:  cr.CanonicalAction.ResourceCount,
		OperationClass: cr.CanonicalAction.OperationClass,
		ScopeClass:     cr.CanonicalAction.ScopeClass,
		CanonVersion:   cr.CanonVersion,
		RetryCount:     retryCount,
	}
}

// Report records the outcome of an operation, matching it to a prescription.
func (s *BenchmarkService) Report(input ReportInput) ReportOutput {
	if input.PrescriptionID == "" {
		return ReportOutput{
			OK:    false,
			Error: &ErrInfo{Code: "invalid_input", Message: "prescription_id is required"},
		}
	}

	// Look up prescription in the new entry store
	if s.evidencePath != "" {
		_, found, err := evidence.FindEntryByID(s.evidencePath, input.PrescriptionID)
		if err != nil {
			return ReportOutput{
				OK:    false,
				Error: &ErrInfo{Code: "evidence_read_failed", Message: err.Error()},
			}
		}
		if !found {
			// Emit unprescribed_action signal entry before rejecting
			sigPayload, _ := json.Marshal(evidence.SignalPayload{
				SignalName: "protocol_violation",
				SubSignal:  "unprescribed_action",
				EntryRefs:  []string{input.PrescriptionID},
				Details:    "report references unknown prescription " + input.PrescriptionID,
			})
			sigActor := s.lastActor
			if input.Actor.ID != "" {
				sigActor = evidence.Actor{Type: input.Actor.Type, ID: input.Actor.ID, Provenance: input.Actor.Origin}
			}
			lastHash, _ := evidence.LastHashAtPath(s.evidencePath)
			sigEntry, buildErr := evidence.BuildEntry(evidence.EntryBuildParams{
				Type:           evidence.EntryTypeSignal,
				TraceID:        evidence.GenerateTraceID(),
				Actor:          sigActor,
				Payload:        sigPayload,
				PreviousHash:   lastHash,
				SpecVersion:    "0.3.0",
				AdapterVersion: version.Version,
				Signer:         s.signer,
			})
			if buildErr == nil {
				_ = evidence.AppendEntryAtPath(s.evidencePath, sigEntry) // best-effort for signal entries
			}
			return ReportOutput{
				OK:    false,
				Error: &ErrInfo{Code: "not_found", Message: "prescription_id not found"},
			}
		}
	}

	// Build report payload
	reportID := ulid.Make().String()
	reportPayload := evidence.ReportPayload{
		ReportID:       reportID,
		PrescriptionID: input.PrescriptionID,
		ExitCode:       input.ExitCode,
		Verdict:        evidence.VerdictFromExitCode(input.ExitCode),
		ExternalRefs:   input.ExternalRefs,
	}
	payloadJSON, err := json.Marshal(reportPayload)
	if err != nil {
		return ReportOutput{
			OK:    false,
			Error: &ErrInfo{Code: "internal_error", Message: "failed to marshal report payload"},
		}
	}

	// Use actor from input if provided, fall back to lastActor from prescribe
	actor := s.lastActor
	if input.Actor.ID != "" {
		actor = evidence.Actor{
			Type:       input.Actor.Type,
			ID:         input.Actor.ID,
			Provenance: input.Actor.Origin,
			InstanceID: input.Actor.InstanceID,
			Version:    input.Actor.Version,
		}
	}

	// Use trace ID from the prescription for correlation
	reportTraceID := s.lastTraceID
	if reportTraceID == "" {
		reportTraceID = evidence.GenerateTraceID()
	}

	lastHash, _ := evidence.LastHashAtPath(s.evidencePath)

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypeReport,
		SessionID:      input.SessionID,
		TraceID:        reportTraceID,
		SpanID:         input.SpanID,
		ParentSpanID:   input.ParentSpanID,
		Actor:          actor,
		ArtifactDigest: evidence.FormatDigest(input.ArtifactDigest),
		Payload:        payloadJSON,
		PreviousHash:   lastHash,
		SpecVersion:    "0.3.0",
		AdapterVersion: version.Version,
		Signer:         s.signer,
	})
	if err != nil {
		return ReportOutput{
			OK:    false,
			Error: &ErrInfo{Code: "internal_error", Message: err.Error()},
		}
	}

	// Write to evidence store
	if s.evidencePath != "" {
		if err := evidence.AppendEntryAtPath(s.evidencePath, entry); err != nil {
			return ReportOutput{
				OK:    false,
				Error: &ErrInfo{Code: "evidence_write_failed", Message: err.Error()},
			}
		}
	}

	return ReportOutput{
		OK:       true,
		ReportID: entry.EntryID,
	}
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
