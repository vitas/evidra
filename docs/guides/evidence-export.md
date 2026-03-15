# Exporting Evidence for Sharing

## Overview

`evidra export` creates anonymized evidence bundles that are safe to share
in GitHub issues, benchmark reports, or with the evidra team. All behavioral
signals and scores are preserved; all identifying information is replaced
with deterministic hashes.

## Quick Start

```bash
# Export your latest evidence (anonymized by default)
evidra export --evidence-dir ~/.evidra/evidence

# Export a specific run from infra-bench
evidra export \
  --evidence-dir runs/e2e/broken-deployment-sonnet/evidence/<session-dir> \
  --include-scorecard

# Export with custom output path
evidra export \
  --evidence-dir /path/to/evidence \
  --output my-issue-bundle
```

## What Gets Exported

The export produces a directory with:

```
evidence-export-<timestamp>/
  manifest.json      # bundle metadata (version, entry count, anonymization)
  evidence.jsonl     # anonymized evidence entries
  metadata.json      # summary (operations, signals, actors, time range)
  scorecard.json     # optional: evidra scorecard output
```

## What Is Anonymized

| Original | Anonymized | Example |
|----------|-----------|---------|
| Namespace names | Hashed | `payments-prod` → `ns-a1b2c3d4` |
| Resource names | Hashed | `api-gateway` → `res-e5f6a7b8` |
| Actor ID | Hashed | `claude-code` → `actor-9c0d1e2f` |
| Session/trace IDs | Hashed | `sess-abc...` → `sess-c1d2e3f4` |
| Scope dimension values | Hashed | `prod-us-1` → `dim-7a8b9c0d` |
| Signatures | Stripped | removed entirely |
| Hash chain | Stripped | removed entirely |
| Decision reasons | Redacted | `[redacted]` |

## What Is Preserved

| Data | Why it's safe |
|------|---------------|
| Tool name (kubectl, terraform) | Generic, no PII |
| Operation (apply, delete) | Generic |
| Operation class (mutate, destroy) | Normalized |
| Scope class (production, staging) | Normalized |
| Resource kind (Deployment, Service) | Generic |
| Resource count | Numeric |
| All digests (artifact, intent, shape) | Already hashes |
| Risk inputs and risk level | Risk assessment |
| Risk tags (k8s.run_as_root) | Generic tags |
| Timestamps | Temporal pattern needed for signals |
| Verdicts (success, failure, declined) | Outcome |
| Exit codes | Numeric |
| Signal names and counts | Behavioral data |
| Scores and bands | Assessment data |
| Version metadata | Reproducibility |

## Use Cases

### Report an Issue

```bash
# Something looks wrong with scoring
evidra export --evidence-dir ~/.evidra/evidence --output issue-evidence
# Attach the directory or zip it to a GitHub issue
tar czf issue-evidence.tar.gz issue-evidence/
```

### Share Benchmark Results

```bash
# After running infra-bench
evidra export \
  --evidence-dir runs/e2e/broken-deployment-sonnet/evidence/<session> \
  --include-scorecard \
  --output benchmark-broken-deployment
```

### Contribute to Community Dataset

```bash
# Export all evidence from a benchmark run
for session in runs/e2e/*/evidence/*/; do
  name=$(basename $(dirname $(dirname "$session")))
  evidra export --evidence-dir "$session" --output "dataset/$name"
done
```

## CLI Reference

```
evidra export [flags]

Flags:
  --evidence-dir string     Evidence directory to export (required)
  --output string           Output directory (default: evidence-export-<timestamp>)
  --anonymize               Anonymize identifiers (default: true)
  --include-scorecard       Include scorecard.json if available
```

## How Anonymization Works

Each export generates a random salt. All identifiers are hashed with
`SHA256(salt + prefix + original)[:8]`. This means:

- **Same original → same hash within one export** (correlations preserved)
- **Different exports use different salts** (can't cross-correlate between exports)
- **8 hex chars** — enough for uniqueness within one chain, not enough to
  brute-force original values

The salt hint (first 4 bytes of `SHA256(salt)`) is stored in `manifest.json`
so the evidra team can distinguish exports from the same user without knowing
the actual salt.

## For the Evidra Team

When you receive an anonymized bundle:

```bash
# Unpack
tar xzf issue-evidence.tar.gz

# Check manifest
cat evidence-export-*/manifest.json

# Score the anonymized evidence
evidra scorecard --evidence-dir evidence-export-*/ --ttl 1s

# Explain signals
evidra explain --evidence-dir evidence-export-*/ --ttl 1s
```

The anonymized evidence produces the exact same signals and scores as the
original — only identifiers change, not behavioral structure.

## For Contributors

### Adding Anonymized Examples to Tests

If you want to contribute test cases based on real evidence:

1. Export your evidence: `evidra export --evidence-dir <path>`
2. Verify no sensitive data: review `evidence.jsonl` manually
3. Submit the bundle as a test fixture

### Building on the Export Package

```go
import "samebits.com/evidra/pkg/export"

// Create an anonymizer
anon := export.NewAnonymizer()

// Anonymize a single entry
anonEntry := anon.AnonymizeEntry(originalEntry)

// Full export
err := export.Export(export.Options{
    EvidenceDir: "/path/to/evidence",
    OutputDir:   "output-bundle",
    Anonymize:   true,
    IncludeScorecard: true,
})
```

## Security Notes

- The salt is random per export, not derived from user identity
- Signatures are stripped — anonymized bundles are not cryptographically verifiable
- Raw artifact content is never included — only digests
- Decision context reasons are redacted (may contain operational details)
- Timestamps are preserved — if timing correlation is a concern, manual
  review is recommended before sharing
