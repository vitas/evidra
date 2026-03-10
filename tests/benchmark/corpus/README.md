# Benchmark Corpus

This directory is the raw artifact corpus used to promote benchmark cases.

It is not the primary real-world e2e artifact root. That role now belongs to:

- `tests/artifacts/real/`
- `tests/artifacts/catalog.yaml`

Rules:

- corpus files are append-only; do not mutate files in place
- if an artifact needs changes, create a new file with a version suffix
- every promoted case must reference corpus provenance through `source_refs`
- if a benchmark artifact is also useful for acceptance testing, prefer
  referencing the shared artifact catalog instead of creating a second unrelated
  fixture copy

This repository currently uses a `limited-contract-baseline` dataset label, so
corpus imports in this phase remain intentionally limited and must be marked as
such in case metadata.
