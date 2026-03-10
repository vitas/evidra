package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/config"
	ievsigner "samebits.com/evidra-benchmark/internal/evidence"
	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/internal/signal"
	"samebits.com/evidra-benchmark/pkg/evidence"
	"samebits.com/evidra-benchmark/pkg/mode"
)

func newLifecycleServiceForCommand(evidenceDir, signingKey, signingKeyPath, signingMode string) (*lifecycle.Service, string, evidence.Signer, error) {
	writeMode, err := config.ResolveEvidenceWriteMode("")
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolve evidence write mode: %w", err)
	}

	signer, err := resolveSigner(signingKey, signingKeyPath, signingMode)
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolve signer: %w", err)
	}

	evidencePath := resolveEvidencePath(evidenceDir)
	svc := lifecycle.NewService(lifecycle.Options{
		EvidencePath:     evidencePath,
		Signer:           signer,
		BestEffortWrites: writeMode == config.EvidenceWriteModeBestEffort,
	})
	return svc, evidencePath, signer, nil
}

func parseCanonicalActionFlag(raw string) (*canon.CanonicalAction, error) {
	if raw == "" {
		return nil, nil
	}

	preCanon := &canon.CanonicalAction{}
	if err := json.Unmarshal([]byte(raw), preCanon); err != nil {
		return nil, fmt.Errorf("parse --canonical-action: %w", err)
	}
	return preCanon, nil
}

func parseExternalRefsFlag(raw string) ([]evidence.ExternalRef, error) {
	if raw == "" {
		return nil, nil
	}

	var externalRefs []evidence.ExternalRef
	if err := json.Unmarshal([]byte(raw), &externalRefs); err != nil {
		return nil, fmt.Errorf("parse --external-refs: %w", err)
	}
	return externalRefs, nil
}

// resolveSigner creates a Signer from explicit flags or environment variables.
// Returns an error when mode is strict and no key is configured.
func resolveSigner(keyBase64, keyPath, modeRaw string) (evidence.Signer, error) {
	mode, err := config.ResolveSigningMode(modeRaw)
	if err != nil {
		return nil, err
	}

	if keyBase64 == "" {
		keyBase64 = strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY"))
	}
	if keyPath == "" {
		keyPath = strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY_PATH"))
	}

	noKey := keyBase64 == "" && keyPath == ""
	if noKey && mode == config.SigningModeStrict {
		return nil, fmt.Errorf("signing key required in strict mode: set --signing-key, --signing-key-path, EVIDRA_SIGNING_KEY, EVIDRA_SIGNING_KEY_PATH, or use --signing-mode optional")
	}

	s, err := ievsigner.NewSigner(ievsigner.SignerConfig{
		KeyBase64: keyBase64,
		KeyPath:   keyPath,
		DevMode:   noKey && mode == config.SigningModeOptional,
	})
	if err != nil {
		return nil, fmt.Errorf("resolveSigner: %w", err)
	}
	return s, nil
}

func resolveEvidencePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := strings.TrimSpace(os.Getenv("EVIDRA_EVIDENCE_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".evidra", "evidence")
	}
	return filepath.Join(home, ".evidra", "evidence")
}

func filterEntries(entries []evidence.EvidenceEntry, actor, period, sessionID string) []evidence.EvidenceEntry {
	cutoff := parsePeriodCutoff(period)
	var filtered []evidence.EvidenceEntry
	for _, e := range entries {
		if actor != "" && e.Actor.ID != actor {
			continue
		}
		if !cutoff.IsZero() && e.Timestamp.Before(cutoff) {
			continue
		}
		if sessionID != "" && e.SessionID != sessionID {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

func filterSignalEntriesByToolAndScope(entries []signal.Entry, tool, scope string) []signal.Entry {
	if tool == "" && scope == "" {
		return entries
	}

	filtered := make([]signal.Entry, 0, len(entries))
	for _, entry := range entries {
		if tool != "" && entry.Tool != tool {
			continue
		}
		if scope != "" && entry.ScopeClass != scope {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func parsePeriodCutoff(period string) time.Time {
	if period == "" {
		return time.Time{}
	}
	now := time.Now().UTC()
	if len(period) < 2 {
		return time.Time{}
	}
	unit := period[len(period)-1]
	val := 0
	_, _ = fmt.Sscanf(period[:len(period)-1], "%d", &val)
	if val <= 0 {
		return time.Time{}
	}
	switch unit {
	case 'd':
		return now.AddDate(0, 0, -val)
	case 'h':
		return now.Add(-time.Duration(val) * time.Hour)
	default:
		return time.Time{}
	}
}

func countPrescriptions(entries []signal.Entry) int {
	count := 0
	for _, entry := range entries {
		if entry.IsPrescription {
			count++
		}
	}
	return count
}

func writeJSON(stdout, stderr io.Writer, context string, payload interface{}) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", context, err)
		return 1
	}
	return 0
}

// forwardEvidence resolves the operating mode and, if online, best-effort
// forwards session evidence entries to the Evidra API.
func forwardEvidence(url, apiKey string, offline, fallbackOffline bool, timeout time.Duration, evidencePath, sessionID string, stderr io.Writer) {
	fallbackPolicy := ""
	if fallbackOffline {
		fallbackPolicy = "offline"
	}
	if v := os.Getenv("EVIDRA_FALLBACK"); v != "" && fallbackPolicy == "" {
		fallbackPolicy = v
	}

	resolved, err := mode.Resolve(mode.Config{
		URL:            url,
		APIKey:         apiKey,
		FallbackPolicy: fallbackPolicy,
		ForceOffline:   offline,
		Timeout:        timeout,
	})
	if err != nil {
		fmt.Fprintf(stderr, "warning: mode resolve: %v\n", err)
		return
	}
	if !resolved.IsOnline {
		return
	}

	entries, err := evidence.ReadAllEntriesAtPath(evidencePath)
	if err != nil {
		fmt.Fprintf(stderr, "warning: read evidence for forwarding: %v\n", err)
		return
	}

	var toForward []json.RawMessage
	for _, entry := range entries {
		if sessionID != "" && entry.SessionID != sessionID {
			continue
		}
		raw, marshalErr := json.Marshal(entry)
		if marshalErr != nil {
			continue
		}
		toForward = append(toForward, raw)
	}
	if len(toForward) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if len(toForward) == 1 {
		if _, fwdErr := resolved.Client.Forward(ctx, toForward[0]); fwdErr != nil {
			fmt.Fprintf(stderr, "warning: forward evidence: %v\n", fwdErr)
		}
	} else {
		if _, fwdErr := resolved.Client.Batch(ctx, toForward); fwdErr != nil {
			fmt.Fprintf(stderr, "warning: batch forward evidence: %v\n", fwdErr)
		}
	}
}
