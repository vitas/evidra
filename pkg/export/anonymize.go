// Package export provides anonymized evidence bundle creation.
package export

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/pkg/evidence"
)

// Anonymizer replaces identifying information with deterministic hashes.
// Same input + same salt → same output, so correlations within one export
// are preserved. Different exports use different salts.
type Anonymizer struct {
	salt  []byte
	cache map[string]string
}

// NewAnonymizer creates an Anonymizer with a random salt.
func NewAnonymizer() *Anonymizer {
	salt := make([]byte, 16)
	rand.Read(salt)
	return &Anonymizer{salt: salt, cache: make(map[string]string)}
}

// Hash returns a deterministic 8-char hex hash of the input.
func (a *Anonymizer) Hash(prefix, original string) string {
	if original == "" {
		return ""
	}
	key := prefix + ":" + original
	if cached, ok := a.cache[key]; ok {
		return cached
	}
	h := sha256.New()
	h.Write(a.salt)
	h.Write([]byte(key))
	result := prefix + "-" + hex.EncodeToString(h.Sum(nil))[:8]
	a.cache[key] = result
	return result
}

// AnonymizeEntry returns a copy of the entry with identifiers replaced.
func (a *Anonymizer) AnonymizeEntry(entry evidence.EvidenceEntry) evidence.EvidenceEntry {
	out := entry

	// Anonymize actor
	out.Actor = evidence.Actor{
		Type:         entry.Actor.Type, // keep: generic (agent, cli, automation)
		ID:           a.Hash("actor", entry.Actor.ID),
		Provenance:   a.Hash("origin", entry.Actor.Provenance),
		InstanceID:   a.Hash("inst", entry.Actor.InstanceID),
		Version:      entry.Actor.Version,      // keep: software version
		SkillVersion: entry.Actor.SkillVersion, // keep: contract version
	}

	// Anonymize correlation IDs
	out.SessionID = a.Hash("sess", entry.SessionID)
	out.OperationID = a.Hash("op", entry.OperationID)
	out.TraceID = a.Hash("trace", entry.TraceID)
	out.SpanID = a.Hash("span", entry.SpanID)
	out.ParentSpanID = a.Hash("span", entry.ParentSpanID)
	out.TenantID = a.Hash("tenant", entry.TenantID)

	// Strip signatures and hash chain (can't verify after anonymization)
	out.Signature = ""
	out.PreviousHash = ""
	out.Hash = ""

	// Anonymize scope dimensions values (keep keys)
	if len(entry.ScopeDimensions) > 0 {
		out.ScopeDimensions = make(map[string]string, len(entry.ScopeDimensions))
		for k, v := range entry.ScopeDimensions {
			out.ScopeDimensions[k] = a.Hash("dim", v)
		}
	}

	// Anonymize payload (type-specific)
	out.Payload = a.anonymizePayload(entry.Type, entry.Payload)

	return out
}

func (a *Anonymizer) anonymizePayload(entryType evidence.EntryType, payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return payload
	}

	var raw map[string]json.RawMessage
	if json.Unmarshal(payload, &raw) != nil {
		return json.RawMessage(`{}`) // strip unparseable payloads
	}

	switch entryType {
	case evidence.EntryTypePrescribe:
		a.anonymizePrescribePayload(raw)
	case evidence.EntryTypeReport:
		a.anonymizeReportPayload(raw)
	case evidence.EntryTypeSignal:
		// Keep signal payloads — they contain signal_name, count, details
		// but strip entry_ref (points to real entry IDs)
		if _, ok := raw["entry_ref"]; ok {
			raw["entry_ref"], _ = json.Marshal(a.Hash("ref", string(raw["entry_ref"])))
		}
	default:
		// Strip unknown payload types entirely
		return json.RawMessage(`{}`)
	}

	result, err := json.Marshal(raw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return result
}

func (a *Anonymizer) anonymizePrescribePayload(raw map[string]json.RawMessage) {
	// Anonymize canonical_action resource identities
	if caRaw, ok := raw["canonical_action"]; ok {
		var ca canon.CanonicalAction
		if json.Unmarshal(caRaw, &ca) == nil {
			ca = a.anonymizeCanonicalAction(ca)
			raw["canonical_action"], _ = json.Marshal(ca)
		}
	}

	// Keep: prescription_id (already a generated ID, but anonymize for safety)
	if v, ok := raw["prescription_id"]; ok {
		var id string
		if json.Unmarshal(v, &id) == nil {
			raw["prescription_id"], _ = json.Marshal(a.Hash("rx", id))
		}
	}

	// Keep risk_inputs, effective_risk, risk_tags — no PII
	// Keep artifact_digest, intent_digest — already hashes
}

func (a *Anonymizer) anonymizeReportPayload(raw map[string]json.RawMessage) {
	// Anonymize prescription_id reference
	if v, ok := raw["prescription_id"]; ok {
		var id string
		if json.Unmarshal(v, &id) == nil {
			raw["prescription_id"], _ = json.Marshal(a.Hash("rx", id))
		}
	}
	// Anonymize report_id
	if v, ok := raw["report_id"]; ok {
		var id string
		if json.Unmarshal(v, &id) == nil {
			raw["report_id"], _ = json.Marshal(a.Hash("rpt", id))
		}
	}

	// Keep: verdict, exit_code, signal_summary, score — no PII
	// Strip: decision_context.reason (may contain sensitive text)
	if _, ok := raw["decision_context"]; ok {
		var dc map[string]json.RawMessage
		if json.Unmarshal(raw["decision_context"], &dc) == nil {
			if _, hasReason := dc["reason"]; hasReason {
				dc["reason"], _ = json.Marshal("[redacted]")
				raw["decision_context"], _ = json.Marshal(dc)
			}
		}
	}
}

func (a *Anonymizer) anonymizeCanonicalAction(ca canon.CanonicalAction) canon.CanonicalAction {
	// Keep: tool, operation, operation_class, scope_class, resource_count, resource_shape_hash
	// Anonymize: resource identity names and namespaces
	for i := range ca.ResourceIdentity {
		ca.ResourceIdentity[i].Name = a.Hash("res", ca.ResourceIdentity[i].Name)
		ca.ResourceIdentity[i].Namespace = a.Hash("ns", ca.ResourceIdentity[i].Namespace)
		// Keep: Kind, APIVersion (generic, no PII)
	}
	return ca
}

// SaltHint returns a short non-reversible identifier for this export's salt.
func (a *Anonymizer) SaltHint() string {
	h := sha256.Sum256(a.salt)
	return hex.EncodeToString(h[:4])
}

// EntryID anonymizes an entry ID.
func (a *Anonymizer) EntryID(id string) string {
	return a.Hash("eid", id)
}
