# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| 0.4.x | Yes |
| < 0.4 | No |

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

## Scope

Security-relevant areas in Evidra include:
- Evidence chain integrity (hash-linking, signatures)
- Ed25519 signing key handling
- File-based locking and concurrent access
- SARIF parser input handling
- `evidra run` local command execution wrapper

## Command Execution Boundary

`evidra run` executes a local command and records evidence around that execution.
Evidra does not sandbox the wrapped command.

Treat `evidra run` with the same trust model as direct shell execution. Evidra
records and analyzes the command; it does not contain, restrict, or make the
wrapped process safe.
