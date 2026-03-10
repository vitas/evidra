package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"samebits.com/evidra-benchmark/internal/assessment"
	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

type reportFlags struct {
	prescriptionID string
	exitCode       int
	evidenceDir    string
	scoringProfile string
	actorID        string
	artifactDigest string
	externalRefs   string
	sessionID      string
	operationID    string
	signingKey     string
	signingKeyPath string
	signingMode    string
}

type reportCommand struct {
	service      *lifecycle.Service
	evidencePath string
	input        lifecycle.ReportInput
}

func cmdReport(args []string, stdout, stderr io.Writer) int {
	opts, code := parseReportFlags(args, stderr)
	if code != 0 {
		return code
	}

	cmd, err := prepareReportCommand(opts)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	profile, err := resolveCommandScoringProfile(opts.scoringProfile)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	reportOut, err := cmd.service.Report(context.Background(), cmd.input)
	if err != nil {
		if lifecycle.ErrorCode(err) == lifecycle.ErrCodeNotFound {
			fmt.Fprintf(stderr, "prescription %s not found in evidence\n", opts.prescriptionID)
			return 1
		}
		fmt.Fprintf(stderr, "report: %v\n", err)
		return 1
	}

	result := map[string]interface{}{
		"ok":              true,
		"report_id":       reportOut.ReportID,
		"prescription_id": reportOut.PrescriptionID,
		"exit_code":       opts.exitCode,
		"verdict":         evidence.VerdictFromExitCode(opts.exitCode),
	}
	snapshot, err := assessment.BuildAtPathWithProfile(cmd.evidencePath, reportOut.SessionID, profile)
	if err != nil {
		fmt.Fprintf(stderr, "report assessment: %v\n", err)
		return 1
	}
	result["score"] = snapshot.Score
	result["score_band"] = snapshot.ScoreBand
	result["scoring_profile_id"] = snapshot.ScoringProfileID
	result["signal_summary"] = snapshot.SignalSummary
	result["basis"] = snapshot.Basis
	result["confidence"] = snapshot.Confidence
	return writeJSON(stdout, stderr, "encode report", result)
}

func parseReportFlags(args []string, stderr io.Writer) (reportFlags, int) {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prescriptionFlag := fs.String("prescription", "", "Prescription event ID")
	exitCodeFlag := fs.Int("exit-code", 0, "Exit code of the operation")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	scoringProfileFlag := fs.String("scoring-profile", "", "Path to scoring profile JSON")
	actorFlag := fs.String("actor", "", "Actor ID")
	artifactDigestFlag := fs.String("artifact-digest", "", "Artifact digest for drift detection")
	externalRefsFlag := fs.String("external-refs", "", "External references JSON array (e.g. '[{\"type\":\"github_run\",\"id\":\"123\"}]')")
	sessionIDFlag := fs.String("session-id", "", "Session/run boundary ID")
	operationIDFlag := fs.String("operation-id", "", "Operation identifier")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	if err := fs.Parse(args); err != nil {
		return reportFlags{}, 2
	}
	if *prescriptionFlag == "" {
		fmt.Fprintln(stderr, "report requires --prescription")
		return reportFlags{}, 2
	}

	return reportFlags{
		prescriptionID: *prescriptionFlag,
		exitCode:       *exitCodeFlag,
		evidenceDir:    *evidenceFlag,
		scoringProfile: *scoringProfileFlag,
		actorID:        *actorFlag,
		artifactDigest: *artifactDigestFlag,
		externalRefs:   *externalRefsFlag,
		sessionID:      *sessionIDFlag,
		operationID:    *operationIDFlag,
		signingKey:     *signingKeyFlag,
		signingKeyPath: *signingKeyPathFlag,
		signingMode:    *signingModeFlag,
	}, 0
}

func prepareReportCommand(opts reportFlags) (reportCommand, error) {
	svc, evidencePath, _, err := newLifecycleServiceForCommand(opts.evidenceDir, opts.signingKey, opts.signingKeyPath, opts.signingMode)
	if err != nil {
		return reportCommand{}, err
	}

	externalRefs, err := parseExternalRefsFlag(opts.externalRefs)
	if err != nil {
		return reportCommand{}, err
	}

	actorID := opts.actorID
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	return reportCommand{
		service:      svc,
		evidencePath: evidencePath,
		input: lifecycle.ReportInput{
			PrescriptionID: opts.prescriptionID,
			ExitCode:       opts.exitCode,
			ArtifactDigest: opts.artifactDigest,
			Actor:          actor,
			ExternalRefs:   externalRefs,
			SessionID:      opts.sessionID,
			OperationID:    opts.operationID,
		},
	}, nil
}
