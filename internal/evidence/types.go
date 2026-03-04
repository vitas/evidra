package evidence

import "time"

// DecisionRecord is the policy decision embedded in an evidence record.
// Field names match the signing payload keys.
type DecisionRecord struct {
	Allow     bool     `json:"allow"`
	RiskLevel string   `json:"risk_level"`
	Reason    string   `json:"reason"`
	Reasons   []string `json:"reasons,omitempty"`
	Hints     []string `json:"hints,omitempty"`
	Hits      []string `json:"hits,omitempty"`
	RuleIDs   []string `json:"rule_ids,omitempty"`
}

// ActorRecord identifies who initiated the invocation.
type ActorRecord struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Origin string `json:"origin"`
}

// EvidenceRecord is a signed evidence record returned by the API.
// The server signs and returns this in the response body. It is never stored server-side.
//
// Signing payload field order (evidra.v1):
//
//	event_id, timestamp, server_id, tenant_id, environment,
//	actor.type, actor.id, actor.origin, tool, operation, input_hash,
//	policy_ref, bundle_revision, profile_name,
//	allow, risk_level, reason, reasons, hints, hits, rule_ids
type EvidenceRecord struct {
	EventID        string         `json:"event_id"`
	Timestamp      time.Time      `json:"timestamp"`
	ServerID       string         `json:"server_id"`
	TenantID       string         `json:"tenant_id"`
	Environment    string         `json:"environment,omitempty"`
	Actor          ActorRecord    `json:"actor"`
	Tool           string         `json:"tool"`
	Operation      string         `json:"operation"`
	InputHash      string         `json:"input_hash"`
	PolicyRef      string         `json:"policy_ref"`
	BundleRevision string         `json:"bundle_revision,omitempty"`
	ProfileName    string         `json:"profile_name,omitempty"`
	Decision       DecisionRecord `json:"decision"`
	Signature      string         `json:"signature"`
	SigningPayload string         `json:"signing_payload"`
}
