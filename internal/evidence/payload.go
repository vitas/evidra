package evidence

import (
	"fmt"
	"strconv"
	"strings"
)

// BuildSigningPayload constructs the deterministic text payload for an evidence record.
//
// Format: version prefix "evidra.v1\n" followed by fields as "key=value\n" in fixed order.
// List fields use length-prefixed encoding via lengthPrefixedJoin.
//
// The field order is:
//
//	event_id, timestamp, server_id, tenant_id, environment,
//	actor.type, actor.id, actor.origin, tool, operation, input_hash,
//	policy_ref, bundle_revision, profile_name,
//	allow, risk_level, reason, reasons, hints, hits, rule_ids
func BuildSigningPayload(rec *EvidenceRecord) string {
	var b strings.Builder
	b.Grow(512)

	b.WriteString("evidra.v1\n")

	writeField(&b, "event_id", rec.EventID)
	writeField(&b, "timestamp", rec.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z"))
	writeField(&b, "server_id", rec.ServerID)
	writeField(&b, "tenant_id", rec.TenantID)
	writeField(&b, "environment", rec.Environment)
	writeField(&b, "actor.type", rec.Actor.Type)
	writeField(&b, "actor.id", rec.Actor.ID)
	writeField(&b, "actor.origin", rec.Actor.Origin)
	writeField(&b, "tool", rec.Tool)
	writeField(&b, "operation", rec.Operation)
	writeField(&b, "input_hash", rec.InputHash)
	writeField(&b, "policy_ref", rec.PolicyRef)
	writeField(&b, "bundle_revision", rec.BundleRevision)
	writeField(&b, "profile_name", rec.ProfileName)
	writeField(&b, "allow", strconv.FormatBool(rec.Decision.Allow))
	writeField(&b, "risk_level", rec.Decision.RiskLevel)
	writeField(&b, "reason", rec.Decision.Reason)
	writeListField(&b, "reasons", rec.Decision.Reasons)
	writeListField(&b, "hints", rec.Decision.Hints)
	writeListField(&b, "hits", rec.Decision.Hits)
	writeListField(&b, "rule_ids", rec.Decision.RuleIDs)

	return b.String()
}

func writeField(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(value)
	b.WriteByte('\n')
}

func writeListField(b *strings.Builder, key string, values []string) {
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(lengthPrefixedJoin(values))
	b.WriteByte('\n')
}

// lengthPrefixedJoin encodes a string slice as a length-prefixed, comma-separated value.
//
// Each element is written as "len:value" where len is the byte length of value.
// Elements are separated by commas. An empty slice produces an empty string.
//
// Examples:
//
//	[]                        -> ""
//	["foo"]                   -> "3:foo"
//	["foo", "hello,world"]    -> "3:foo,11:hello,world"
//	["", "a"]                 -> "0:,1:a"
//	["cafe"]                  -> "5:cafe"
func lengthPrefixedJoin(values []string) string {
	if len(values) == 0 {
		return ""
	}

	var b strings.Builder
	// Pre-calculate capacity: each entry needs digits + colon + value + comma.
	size := 0
	for _, v := range values {
		size += len(strconv.Itoa(len(v))) + 1 + len(v) + 1 // digits + ':' + value + ','
	}
	b.Grow(size)

	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%d:%s", len(v), v)
	}

	return b.String()
}
