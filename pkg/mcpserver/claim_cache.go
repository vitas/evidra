package mcpserver

import (
	"strings"
	"time"

	"samebits.com/evidra/pkg/evidence"
)

// autoPrescribeOrigin is the actor origin recorded for server-derived
// prescriptions on observed mutations.
const autoPrescribeOrigin = "evidra:auto"

// claimRecord remembers a model-issued prescription so run_command can pair
// the executed mutation with the claim that preceded it. This closes the
// claim↔action loop without a second prescription: the report references the
// model's prescription id, and only unlinked mutations get an
// AutoPrescribed=true fallback prescription (the unprescribed_mutation
// signal's raw material).
type claimRecord struct {
	prescriptionID string
	claimedAt      time.Time
}

// claimKey is the normalized identity of a claimed action: tool + operation +
// single target resource. Deliberately fuzzy (no artifact digest): exact
// digest equality is the artifact_drift signal's job, while linking only has
// to answer "was this the change the agent said it would make".
type claimKey struct {
	tool       string
	operation  string
	resourceID string
}

const (
	// claimTTL mirrors the prescription TTL enforced by report validation:
	// linking an older claim than that would only trade an auto-prescription
	// for a TTL-expired protocol violation.
	claimTTL     = time.Duration(evidence.DefaultTTLMs) * time.Millisecond
	claimMaxSize = 256
)

func newClaimKey(tool, operation string, resourceID evidence.ResourceID) claimKey {
	return claimKey{
		tool:      strings.ToLower(strings.TrimSpace(tool)),
		operation: strings.ToLower(strings.TrimSpace(operation)),
		resourceID: strings.Join([]string{
			strings.ToLower(resourceID.Kind),
			strings.ToLower(resourceID.Name),
			strings.ToLower(resourceID.Namespace),
		}, "/"),
	}
}

// claimKeyFromPrescribeInput derives the link key for either side of the
// protocol: a model-issued prescribe (resource given as "kind/name" strings)
// or a run_command-derived action (normalized ResourceIdentity). Multi-
// resource and resource-less actions are not linkable and fall through to
// auto-prescribe unchanged.
func claimKeyFromPrescribeInput(in PrescribeInput) (claimKey, bool) {
	tool, operation := in.Tool, in.Operation
	if in.CanonicalAction != nil {
		if len(in.CanonicalAction.ResourceIdentity) == 1 {
			return newClaimKey(
				firstNonEmpty(tool, in.CanonicalAction.Tool),
				firstNonEmpty(operation, in.CanonicalAction.Operation),
				in.CanonicalAction.ResourceIdentity[0],
			), true
		}
		return claimKey{}, false
	}
	if strings.TrimSpace(in.Resource) == "" {
		return claimKey{}, false
	}
	resourceID, err := parseSmartResource(in.Resource, in.Namespace)
	if err != nil {
		return claimKey{}, false
	}
	return newClaimKey(tool, operation, resourceID), true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *MCPService) recordClaim(input PrescribeInput, prescriptionID string) {
	key, ok := claimKeyFromPrescribeInput(input)
	if !ok || prescriptionID == "" {
		return
	}
	now := time.Now()
	s.claimsMu.Lock()
	defer s.claimsMu.Unlock()
	if s.recentClaims == nil {
		s.recentClaims = make(map[claimKey]claimRecord)
	}
	for k, rec := range s.recentClaims {
		if now.Sub(rec.claimedAt) > claimTTL {
			delete(s.recentClaims, k)
		}
	}
	// Bound memory: on overflow keep the freshest entries.
	if len(s.recentClaims) >= claimMaxSize {
		var oldestKey claimKey
		var oldest time.Time
		for k, rec := range s.recentClaims {
			if oldestKey == (claimKey{}) || rec.claimedAt.Before(oldest) {
				oldestKey, oldest = k, rec.claimedAt
			}
		}
		delete(s.recentClaims, oldestKey)
	}
	s.recentClaims[key] = claimRecord{prescriptionID: prescriptionID, claimedAt: now}
}

// takeClaim returns the prescription id of a live unconsumed claim for the
// same normalized action and removes it (one claim authorizes one mutation;
// a retry of the same operation should re-prescribe or show up as a
// distinct unprescribed mutation, never silently reuse a spent claim).
func (s *MCPService) takeClaim(input PrescribeInput) (string, bool) {
	key, ok := claimKeyFromPrescribeInput(input)
	if !ok {
		return "", false
	}
	s.claimsMu.Lock()
	defer s.claimsMu.Unlock()
	rec, found := s.recentClaims[key]
	if !found {
		return "", false
	}
	delete(s.recentClaims, key)
	if time.Since(rec.claimedAt) > claimTTL {
		return "", false
	}
	return rec.prescriptionID, true
}
