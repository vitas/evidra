package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra/internal/signal"
	"samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func newClaimTestHandler(t *testing.T, evidenceDir string) *runCommandHandler {
	t.Helper()
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "kubectl"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &runCommandHandler{
		service: &MCPService{
			evidencePath: evidenceDir,
			signer:       testutil.TestSigner(t),
		},
		allowedPrefixes: defaultAllowedPrefixes,
		blockedSubs:     defaultBlockedSubcommands,
	}
}

func modelClaimInput() PrescribeInput {
	return PrescribeInput{
		Actor:     InputActor{Type: "agent", ID: "kagent-sre", Origin: "kagent"},
		Tool:      "kubectl",
		Operation: "patch",
		Resource:  "deployment/web",
		Namespace: "demo",
	}
}

const patchCommand = "kubectl patch deployment web -n demo --type=merge"

func TestClaimKeyMatchesModelSmartInputAndDerivedCommand(t *testing.T) {
	derived, ok, err := deriveAutoPrescribeInput(patchCommand, "kagent-sre")
	if err != nil || !ok {
		t.Fatalf("deriveAutoPrescribeInput = (%v, %v)", ok, err)
	}
	fromModel, ok := claimKeyFromPrescribeInput(modelClaimInput())
	if !ok {
		t.Fatal("model claim input produced no key")
	}
	fromDerived, ok := claimKeyFromPrescribeInput(derived)
	if !ok {
		t.Fatal("derived claim input produced no key")
	}
	if fromModel != fromDerived {
		t.Fatalf("claim keys differ: model %+v derived %+v", fromModel, fromDerived)
	}
}

func TestRunCommandLinksModelPrescriptionInsteadOfAutoPrescribe(t *testing.T) {
	dir := t.TempDir()
	handler := newClaimTestHandler(t, dir)
	ctx := context.Background()

	claim := handler.service.PrescribeCtx(ctx, modelClaimInput())
	if !claim.OK {
		t.Fatalf("model prescribe failed: %+v", claim.Error)
	}
	out := handler.execute(ctx, RunCommandInput{Command: patchCommand})
	if !out.OK {
		t.Fatalf("run_command failed: %+v", out)
	}

	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2 (one prescription, one report), types: %v",
			len(entries), entryTypes(entries))
	}
	var presc evidence.PrescriptionPayload
	if err := json.Unmarshal(entries[0].Payload, &presc); err != nil {
		t.Fatal(err)
	}
	if presc.AutoPrescribed {
		t.Fatal("linked prescription must not be marked auto")
	}
	if presc.PrescriptionID != claim.PrescriptionID {
		t.Fatalf("prescription id = %s, want model claim id %s", presc.PrescriptionID, claim.PrescriptionID)
	}
	var report evidence.ReportPayload
	if err := json.Unmarshal(entries[1].Payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.PrescriptionID != claim.PrescriptionID {
		t.Fatalf("report references %s, want model claim %s", report.PrescriptionID, claim.PrescriptionID)
	}

	// A clean claim→action pair must produce no unprescribed signal.
	signalEntries, err := signalEntriesFromStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res := signal.DetectUnprescribedMutations(signalEntries); res.Count != 0 {
		t.Fatalf("unprescribed_mutation on linked run = %d, want 0", res.Count)
	}
}

func TestRunCommandWithoutModelClaimWritesFlaggedAutoPrescription(t *testing.T) {
	dir := t.TempDir()
	handler := newClaimTestHandler(t, dir)
	ctx := context.Background()

	out := handler.execute(ctx, RunCommandInput{Command: patchCommand})
	if !out.OK {
		t.Fatalf("run_command failed: %+v", out)
	}
	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	var presc evidence.PrescriptionPayload
	if err := json.Unmarshal(entries[0].Payload, &presc); err != nil {
		t.Fatal(err)
	}
	if !presc.AutoPrescribed {
		t.Fatal("unlinked mutation must be flagged auto_prescribed")
	}

	signalEntries, err := signalEntriesFromStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res := signal.DetectUnprescribedMutations(signalEntries); res.Count != 1 {
		t.Fatalf("unprescribed_mutation = %d, want 1", res.Count)
	}
}

func TestClaimIsConsumedOnce(t *testing.T) {
	dir := t.TempDir()
	handler := newClaimTestHandler(t, dir)
	ctx := context.Background()

	claim := handler.service.PrescribeCtx(ctx, modelClaimInput())
	if !claim.OK {
		t.Fatalf("model prescribe failed: %+v", claim.Error)
	}
	if out := handler.execute(ctx, RunCommandInput{Command: patchCommand}); !out.OK {
		t.Fatalf("first run failed: %+v", out)
	}
	if out := handler.execute(ctx, RunCommandInput{Command: patchCommand}); !out.OK {
		t.Fatalf("second run failed: %+v", out)
	}

	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("entry count = %d, want 4 (claim, report, auto prescription, report)", len(entries))
	}
	var second evidence.PrescriptionPayload
	if err := json.Unmarshal(entries[2].Payload, &second); err != nil {
		t.Fatal(err)
	}
	if !second.AutoPrescribed {
		t.Fatal("retry of the same action without a fresh claim must be auto-prescribed")
	}
}

func signalEntriesFromStore(dir string) ([]signal.Entry, error) {
	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		return nil, err
	}
	out := make([]signal.Entry, 0, len(entries))
	for _, e := range entries {
		se := signal.Entry{
			EventID:   e.EntryID,
			Timestamp: e.Timestamp,
			ActorID:   e.Actor.ID,
		}
		switch e.Type {
		case evidence.EntryTypePrescribe:
			var p evidence.PrescriptionPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return nil, err
			}
			se.IsPrescription = true
			se.AutoPrescribed = p.AutoPrescribed
		case evidence.EntryTypeReport:
			se.IsReport = true
		}
		out = append(out, se)
	}
	return out, nil
}

func entryTypes(entries []evidence.EvidenceEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = string(e.Type)
	}
	return out
}
