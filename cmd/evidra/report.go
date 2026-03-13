package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"samebits.com/evidra/internal/assessment"
	"samebits.com/evidra/internal/lifecycle"
	"samebits.com/evidra/pkg/evidence"
)

type reportFlags struct {
	prescriptionID  string
	verdict         evidence.Verdict
	exitCode        optionalIntFlag
	declineTrigger  string
	declineReason   string
	evidenceDir     string
	scoringProfile  string
	actorID         string
	artifactDigest  string
	externalRefs    string
	sessionID       string
	operationID     string
	signingKey      string
	signingKeyPath  string
	signingMode     string
	url             string
	apiKey          string
	offline         bool
	fallbackOffline bool
	timeout         time.Duration
}

type reportCommand struct {
	service      *lifecycle.Service
	evidencePath string
	input        lifecycle.ReportInput
}

type optionalIntFlag struct {
	set   bool
	value int
}

func (o *optionalIntFlag) String() string {
	if !o.set {
		return ""
	}
	return strconv.Itoa(o.value)
}

func (o *optionalIntFlag) Set(raw string) error {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return err
	}
	o.set = true
	o.value = value
	return nil
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
		"verdict":         string(reportOut.Verdict),
	}
	if reportOut.ExitCode != nil {
		result["exit_code"] = *reportOut.ExitCode
	}
	if reportOut.DecisionContext != nil {
		result["decision_context"] = reportOut.DecisionContext
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
	code = writeJSON(stdout, stderr, "encode report", result)
	if code != 0 {
		return code
	}

	forwardEvidence(opts.url, opts.apiKey, opts.offline, opts.fallbackOffline, opts.timeout, cmd.evidencePath, reportOut.SessionID, stderr)
	return 0
}

func parseReportFlags(args []string, stderr io.Writer) (reportFlags, int) {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prescriptionFlag := fs.String("prescription", "", "Prescription event ID")
	var exitCodeFlag optionalIntFlag
	fs.Var(&exitCodeFlag, "exit-code", "Exit code of the operation")
	verdictFlag := fs.String("verdict", "", "Terminal outcome: success, failure, error, or declined")
	declineTriggerFlag := fs.String("decline-trigger", "", "Decline trigger value for verdict=declined")
	declineReasonFlag := fs.String("decline-reason", "", "Short operational reason for verdict=declined")
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
	urlFlag := fs.String("url", os.Getenv("EVIDRA_URL"), "Evidra API URL")
	apiKeyFlag := fs.String("api-key", os.Getenv("EVIDRA_API_KEY"), "Evidra API key")
	offlineFlag := fs.Bool("offline", false, "Force offline mode")
	fallbackOfflineFlag := fs.Bool("fallback-offline", false, "Fall back to offline on API failure")
	timeoutFlag := fs.Duration("timeout", 30*time.Second, "API request timeout")
	if err := fs.Parse(args); err != nil {
		return reportFlags{}, 2
	}
	if *prescriptionFlag == "" {
		fmt.Fprintln(stderr, "report requires --prescription")
		return reportFlags{}, 2
	}
	verdict := evidence.Verdict(strings.TrimSpace(*verdictFlag))
	if strings.TrimSpace(*declineTriggerFlag) != "" || strings.TrimSpace(*declineReasonFlag) != "" {
		if verdict == "" {
			verdict = evidence.VerdictDeclined
		}
	}
	if verdict == "" {
		fmt.Fprintln(stderr, "report requires --verdict")
		return reportFlags{}, 2
	}
	if !verdict.Valid() {
		fmt.Fprintf(stderr, "invalid verdict %q\n", verdict)
		return reportFlags{}, 2
	}
	if !validateReportVerdict(stderr, verdict, exitCodeFlag, *declineTriggerFlag, *declineReasonFlag) {
		return reportFlags{}, 2
	}

	return reportFlags{
		prescriptionID:  *prescriptionFlag,
		verdict:         verdict,
		exitCode:        exitCodeFlag,
		declineTrigger:  strings.TrimSpace(*declineTriggerFlag),
		declineReason:   strings.TrimSpace(*declineReasonFlag),
		evidenceDir:     *evidenceFlag,
		scoringProfile:  *scoringProfileFlag,
		actorID:         *actorFlag,
		artifactDigest:  *artifactDigestFlag,
		externalRefs:    *externalRefsFlag,
		sessionID:       *sessionIDFlag,
		operationID:     *operationIDFlag,
		signingKey:      *signingKeyFlag,
		signingKeyPath:  *signingKeyPathFlag,
		signingMode:     *signingModeFlag,
		url:             *urlFlag,
		apiKey:          *apiKeyFlag,
		offline:         *offlineFlag,
		fallbackOffline: *fallbackOfflineFlag,
		timeout:         *timeoutFlag,
	}, 0
}

func validateReportVerdict(stderr io.Writer, verdict evidence.Verdict, exitCodeFlag optionalIntFlag, declineTrigger, declineReason string) bool {
	trigger := strings.TrimSpace(declineTrigger)
	reason := strings.TrimSpace(declineReason)

	if verdict == evidence.VerdictDeclined {
		if exitCodeFlag.set {
			fmt.Fprintln(stderr, "declined report must not include --exit-code")
			return false
		}
		if trigger == "" {
			fmt.Fprintln(stderr, "declined report requires --decline-trigger")
			return false
		}
		if reason == "" {
			fmt.Fprintln(stderr, "declined report requires --decline-reason")
			return false
		}
		return true
	}

	if !exitCodeFlag.set {
		fmt.Fprintf(stderr, "report verdict %s requires --exit-code\n", verdict)
		return false
	}
	if trigger != "" || reason != "" {
		fmt.Fprintln(stderr, "decline fields are only valid with --verdict declined")
		return false
	}
	if inferred := evidence.VerdictFromExitCode(exitCodeFlag.value); inferred != verdict {
		fmt.Fprintf(stderr, "report verdict %s does not match --exit-code %d\n", verdict, exitCodeFlag.value)
		return false
	}
	return true
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
	var exitCode *int
	if opts.exitCode.set {
		exitCode = intPtr(opts.exitCode.value)
	}
	var decisionContext *evidence.DecisionContext
	if opts.verdict == evidence.VerdictDeclined {
		decisionContext = &evidence.DecisionContext{
			Trigger: opts.declineTrigger,
			Reason:  opts.declineReason,
		}
	}

	return reportCommand{
		service:      svc,
		evidencePath: evidencePath,
		input: lifecycle.ReportInput{
			PrescriptionID:  opts.prescriptionID,
			Verdict:         opts.verdict,
			ExitCode:        exitCode,
			DecisionContext: decisionContext,
			ArtifactDigest:  opts.artifactDigest,
			Actor:           actor,
			ExternalRefs:    externalRefs,
			SessionID:       opts.sessionID,
			OperationID:     opts.operationID,
		},
	}, nil
}
