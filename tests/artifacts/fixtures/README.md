# Shared Artifact Fixtures

This directory is the single physical home for vendored real-world artifacts
used by Evidra's acceptance workflows.

Rules:

- acceptance tests reference files from this root
- provenance classification belongs in metadata, not in directory ownership
- organize files by artifact family (`k8s/`, `terraform/`, `helm/`, etc.)
- imports come from reviewed upstream sources, not runtime downloads in CI
- prefer adding new shared fixtures here instead of creating duplicate copies in
  test-suite-specific directories

The authoritative inventory for acceptance-facing fixtures remains:

- `tests/artifacts/catalog.yaml`

New real-world acceptance fixtures should be added here and cataloged in
`tests/artifacts/catalog.yaml`.
