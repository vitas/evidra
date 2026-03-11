package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/sarif"
	"samebits.com/evidra-benchmark/pkg/evidence"
	"samebits.com/evidra-benchmark/pkg/version"
)

type importFindingsFlags struct {
	sarifPath      string
	artifactPath   string
	toolVersion    string
	evidenceDir    string
	actorID        string
	sessionID      string
	signingKey     string
	signingKeyPath string
	signingMode    string
}

type importFindingsCommand struct {
	findings       []evidence.FindingPayload
	evidencePath   string
	artifactDigest string
	actor          evidence.Actor
	sessionID      string
	signer         evidence.Signer
}

type findingAppendConfig struct {
	evidencePath   string
	signer         evidence.Signer
	sessionID      string
	operationID    string
	attempt        int
	traceID        string
	actor          evidence.Actor
	artifactDigest string
}

func cmdImportFindings(args []string, stdout, stderr io.Writer) int {
	opts, code := parseImportFindingsFlags(args, stderr)
	if code != 0 {
		return code
	}

	cmd, err := prepareImportFindingsCommand(opts)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	written := appendFindingsAsEvidence(cmd.findings, findingAppendConfig{
		evidencePath:   cmd.evidencePath,
		signer:         cmd.signer,
		sessionID:      cmd.sessionID,
		traceID:        cmd.sessionID,
		actor:          cmd.actor,
		artifactDigest: cmd.artifactDigest,
	}, stderr)

	result := map[string]interface{}{
		"ok":              true,
		"findings_count":  written,
		"artifact_digest": cmd.artifactDigest,
	}
	return writeJSON(stdout, stderr, "encode result", result)
}

func parseImportFindingsFlags(args []string, stderr io.Writer) (importFindingsFlags, int) {
	fs := flag.NewFlagSet("import-findings", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sarifFlag := fs.String("sarif", "", "Path to SARIF scanner report")
	artifactFlag := fs.String("artifact", "", "Path to artifact file (for artifact_digest linking)")
	toolVersionFlag := fs.String("tool-version", "", "Override tool version for all ingested findings")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	actorFlag := fs.String("actor", "", "Actor ID")
	sessionIDFlag := fs.String("session-id", "", "Session/run boundary ID")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	if err := fs.Parse(args); err != nil {
		return importFindingsFlags{}, 2
	}
	if *sarifFlag == "" {
		fmt.Fprintln(stderr, "import-findings requires --sarif")
		return importFindingsFlags{}, 2
	}

	return importFindingsFlags{
		sarifPath:      *sarifFlag,
		artifactPath:   *artifactFlag,
		toolVersion:    *toolVersionFlag,
		evidenceDir:    *evidenceFlag,
		actorID:        *actorFlag,
		sessionID:      *sessionIDFlag,
		signingKey:     *signingKeyFlag,
		signingKeyPath: *signingKeyPathFlag,
		signingMode:    *signingModeFlag,
	}, 0
}

func prepareImportFindingsCommand(opts importFindingsFlags) (importFindingsCommand, error) {
	sessionID := opts.sessionID
	if sessionID == "" {
		sessionID = evidence.GenerateSessionID()
	}

	signer, err := resolveSigner(opts.signingKey, opts.signingKeyPath, opts.signingMode)
	if err != nil {
		return importFindingsCommand{}, fmt.Errorf("resolve signer: %w", err)
	}

	findings, err := loadSARIFFindings(opts.sarifPath, "sarif")
	if err != nil {
		return importFindingsCommand{}, err
	}
	if opts.toolVersion != "" {
		for i := range findings {
			findings[i].ToolVersion = opts.toolVersion
		}
	}

	artifactDigest := ""
	if opts.artifactPath != "" {
		artifactData, err := os.ReadFile(opts.artifactPath)
		if err != nil {
			return importFindingsCommand{}, fmt.Errorf("read artifact: %w", err)
		}
		artifactDigest = canon.ComputeArtifactDigest(artifactData)
	}

	actorID := opts.actorID
	if actorID == "" {
		actorID = "cli"
	}

	return importFindingsCommand{
		findings:       findings,
		evidencePath:   resolveEvidencePath(opts.evidenceDir),
		artifactDigest: artifactDigest,
		actor: evidence.Actor{
			Type:       "cli",
			ID:         actorID,
			Provenance: "cli",
		},
		sessionID: sessionID,
		signer:    signer,
	}, nil
}

func loadSARIFFindings(path, sourceLabel string) ([]evidence.FindingPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sourceLabel, err)
	}

	findings, err := sarif.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", sourceLabel, err)
	}
	return findings, nil
}

func appendFindingsAsEvidence(findings []evidence.FindingPayload, cfg findingAppendConfig, stderr io.Writer) int {
	traceID := cfg.traceID
	if traceID == "" {
		traceID = cfg.sessionID
	}

	written := 0
	for _, finding := range findings {
		findingPayload, _ := json.Marshal(finding)
		lastHash, _ := evidence.LastHashAtPath(cfg.evidencePath)
		entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
			Type:           evidence.EntryTypeFinding,
			SessionID:      cfg.sessionID,
			OperationID:    cfg.operationID,
			Attempt:        cfg.attempt,
			TraceID:        traceID,
			Actor:          cfg.actor,
			ArtifactDigest: cfg.artifactDigest,
			Payload:        findingPayload,
			PreviousHash:   lastHash,
			SpecVersion:    version.SpecVersion,
			AdapterVersion: version.Version,
			ScoringVersion: version.ScoringVersion,
			Signer:         cfg.signer,
		})
		if err != nil {
			fmt.Fprintf(stderr, "warning: build finding entry failed for rule %s: %v\n", finding.RuleID, err)
			continue
		}
		if err := evidence.AppendEntryAtPath(cfg.evidencePath, entry); err != nil {
			fmt.Fprintf(stderr, "warning: write finding entry failed for rule %s: %v\n", finding.RuleID, err)
			continue
		}
		written++
	}
	return written
}
