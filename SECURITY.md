# Security Policy

## Supported Versions

The `main` branch is the actively developed, unreleased Evidra Core line: the
MCP execution-evidence recorder described in [README.md](README.md). Security
work lands there.

Published pre-vNext releases describe a different, legacy product (the
all-in-one DevOps surface retired during vNext) and receive no implied feature
or security support from `main`. If you are affected by a pre-vNext release,
say so in your report; it will be triaged as legacy.

## Reporting a Vulnerability

If you discover a security vulnerability in Evidra, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, email: security@samebits.com

Include:
- Description of the vulnerability
- Steps to reproduce
- Impact assessment
- Suggested fix (if any)

We will acknowledge receipt within 48 hours and provide a timeline for a fix.

## What Evidra Does Not Do

- **Evidra is not a sandbox.** The endpoint forwards tool calls to the upstream
  MCP server it wraps; that server executes whatever it executes, with whatever
  privileges it has. Evidra does not contain, restrict, or make safe the work
  it observes.
- **Evidra is not an external-state verifier.** A successful upstream response
  is not proof of the external outcome. The evidence chain records what was
  declared, observed, and reported — reconciling that with reality is the
  human step.
- **The recorder does not authenticate organizations.** The local Ed25519
  signing key and the HMAC digest key protect chain consistency within the
  directory threat model stated in
  [docs/evidence-format.md](docs/evidence-format.md): anyone who can replace
  the whole evidence directory can replace its history. The keys do not prove
  organizational identity against directory replacement.

## Handling Data

Arguments and results are agent- and upstream-controlled text. Do not place
raw secrets in agent declarations or reports: declared and reported payloads
are stored in the chain as given. Observed arguments leave the process only as
keyed HMAC digests, and observed results are fingerprinted, never stored raw;
see [docs/evidence-format.md](docs/evidence-format.md) for the exact
cryptographic and privacy behavior.
