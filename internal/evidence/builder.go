package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/oklog/ulid/v2"

	"samebits.com/evidra-benchmark/pkg/invocation"
)

// BuilderConfig holds the server-level fields injected into every evidence record.
type BuilderConfig struct {
	ServerID string
	TenantID string
}

// BuildRecord constructs an unsigned EvidenceRecord from a policy decision and tool invocation.
// The Signature and SigningPayload fields are left empty -- the caller signs separately.
func BuildRecord(cfg BuilderConfig, dec Decision, inv invocation.ToolInvocation) (EvidenceRecord, error) {
	inputHash, err := hashInvocation(inv)
	if err != nil {
		return EvidenceRecord{}, fmt.Errorf("evidence.BuildRecord: %w", err)
	}

	return EvidenceRecord{
		EventID:        generateEventID(),
		Timestamp:      time.Now().UTC(),
		ServerID:       cfg.ServerID,
		TenantID:       cfg.TenantID,
		Environment:    inv.Environment,
		Actor:          ActorRecord{Type: inv.Actor.Type, ID: inv.Actor.ID, Origin: inv.Actor.Origin},
		Tool:           inv.Tool,
		Operation:      inv.Operation,
		InputHash:      inputHash,
		PolicyRef:      dec.PolicyRef,
		BundleRevision: dec.BundleRevision,
		ProfileName:    dec.ProfileName,
		Decision: DecisionRecord{
			Allow:     dec.Allow,
			RiskLevel: dec.RiskLevel,
			Reason:    dec.Reason,
			Reasons:   dec.Reasons,
			Hints:     dec.Hints,
			Hits:      dec.Hits,
			RuleIDs:   dec.Hits, // rule_ids derived from hits
		},
	}, nil
}

func generateEventID() string {
	// Use math/rand entropy source -- ULID does not need cryptographic randomness.
	entropy := ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)
	return "evt_" + ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

func hashInvocation(inv invocation.ToolInvocation) (string, error) {
	data, err := json.Marshal(inv)
	if err != nil {
		return "", fmt.Errorf("hashInvocation: marshal: %w", err)
	}
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}
