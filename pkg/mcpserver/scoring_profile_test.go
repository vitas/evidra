package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra/internal/score"
	"samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func TestReport_UsesConfiguredScoringProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	profilePath := writeCustomScoringProfile(t, "custom.mcp-profile")

	server, err := NewServer(Options{
		Name:               "test",
		Version:            "0.0.1",
		EvidencePath:       dir,
		Signer:             testutil.TestSigner(t),
		ScoringProfilePath: profilePath,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	_ = server

	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}
	svc.scoringProfile, err = score.ResolveProfile(profilePath)
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	presc := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "agent-1", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
		SessionID:   "session-mcp-profile",
	})
	if !presc.OK {
		t.Fatalf("prescribe failed: %v", presc.Error)
	}

	report := svc.Report(ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
		SessionID:      "session-mcp-profile",
	})
	if !report.OK {
		t.Fatalf("report failed: %+v", report)
	}
	if report.ScoringProfileID != "custom.mcp-profile" {
		t.Fatalf("scoring_profile_id = %q, want %q", report.ScoringProfileID, "custom.mcp-profile")
	}
}

func writeCustomScoringProfile(t *testing.T, id string) string {
	t.Helper()

	profile, err := score.LoadDefaultProfile()
	if err != nil {
		t.Fatalf("LoadDefaultProfile: %v", err)
	}
	profile.ID = id
	profile.Weights["artifact_drift"] = 0.13
	profile.Weights["protocol_violation"] = 0.42

	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}

	path := filepath.Join(t.TempDir(), id+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}
