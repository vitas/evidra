package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra/pkg/evidence"
)

// PrescribeFullInput is the input schema for the prescribe_full tool.
type PrescribeFullInput struct {
	Actor           InputActor                `json:"actor"`
	Tool            string                    `json:"tool"`
	Operation       string                    `json:"operation"`
	RawArtifact     string                    `json:"raw_artifact"`
	Environment     string                    `json:"environment,omitempty"`
	CanonicalAction *evidence.CanonicalAction `json:"canonical_action,omitempty"`
	SessionID       string                    `json:"session_id,omitempty"`
	OperationID     string                    `json:"operation_id,omitempty"`
	Attempt         int                       `json:"attempt,omitempty"`
	TraceID         string                    `json:"trace_id,omitempty"`
	SpanID          string                    `json:"span_id,omitempty"`
	ParentSpanID    string                    `json:"parent_span_id,omitempty"`
	ScopeDimensions map[string]string         `json:"scope_dimensions,omitempty"`
}

func (h *prescribeFullHandler) Handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input PrescribeFullInput,
) (*mcp.CallToolResult, PrescribeOutput, error) {
	output := h.service.PrescribeCtx(ctx, input.toPrescribeInput())
	return &mcp.CallToolResult{}, output, nil
}

func (input PrescribeFullInput) toPrescribeInput() PrescribeInput {
	return PrescribeInput{
		Actor:           input.Actor,
		Tool:            input.Tool,
		Operation:       input.Operation,
		RawArtifact:     input.RawArtifact,
		Environment:     input.Environment,
		CanonicalAction: input.CanonicalAction,
		SessionID:       input.SessionID,
		OperationID:     input.OperationID,
		Attempt:         input.Attempt,
		TraceID:         input.TraceID,
		SpanID:          input.SpanID,
		ParentSpanID:    input.ParentSpanID,
		ScopeDimensions: input.ScopeDimensions,
	}
}
