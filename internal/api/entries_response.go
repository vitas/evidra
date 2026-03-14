package api

import (
	"encoding/json"
	"time"

	"samebits.com/evidra/internal/store"
)

type entryAPIResponse struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Tool      string    `json:"tool,omitempty"`
	Operation string    `json:"operation,omitempty"`
	Scope     string    `json:"scope,omitempty"`
	RiskLevel string    `json:"risk_level,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	Verdict   string    `json:"verdict,omitempty"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toEntryAPIResponse(e store.StoredEntry) entryAPIResponse {
	resp := entryAPIResponse{
		ID:        e.ID,
		Type:      e.EntryType,
		CreatedAt: e.CreatedAt,
	}

	// Extract fields from Payload JSON.
	var payload struct {
		Tool          string `json:"tool"`
		Operation     string `json:"operation"`
		EffectiveRisk string `json:"effective_risk"`
		RiskLevel     string `json:"risk_level"`
		Actor         struct {
			ID string `json:"id"`
		} `json:"actor"`
		CanonicalAction struct {
			Tool           string `json:"tool"`
			Operation      string `json:"operation"`
			OperationClass string `json:"operation_class"`
			ScopeClass     string `json:"scope_class"`
		} `json:"canonical_action"`
		Verdict  string `json:"verdict"`
		ExitCode *int   `json:"exit_code"`
		Scope    string `json:"scope"`
	}
	_ = json.Unmarshal(e.Payload, &payload)

	resp.Actor = payload.Actor.ID
	resp.Verdict = payload.Verdict
	resp.ExitCode = payload.ExitCode
	resp.RiskLevel = payload.EffectiveRisk
	if resp.RiskLevel == "" {
		resp.RiskLevel = payload.RiskLevel
	}

	// Prefer canonical_action fields, fall back to top-level.
	resp.Tool = payload.CanonicalAction.Tool
	if resp.Tool == "" {
		resp.Tool = payload.Tool
	}
	resp.Operation = payload.CanonicalAction.Operation
	if resp.Operation == "" {
		resp.Operation = payload.Operation
	}
	resp.Scope = payload.CanonicalAction.ScopeClass
	if resp.Scope == "" {
		resp.Scope = payload.Scope
	}

	return resp
}

func toEntryAPIResponses(entries []store.StoredEntry) []entryAPIResponse {
	out := make([]entryAPIResponse, len(entries))
	for i, e := range entries {
		out[i] = toEntryAPIResponse(e)
	}
	return out
}
