# Evidra Architecture

Status: **vNext**, branch `vnext/mcp-recorder`. The design authority is
[`system-design/vnext-mcp-recorder.md`](system-design/vnext-mcp-recorder.md) (the plan,
§§ numbered below); this file is the map, not the spec. What the pre-vNext product was —
risk scoring, the assessment pipeline, hosted API, scorecards, direct MCP tools — is
deleted per §43, and [`system-design/vnext-prune-record.md`](system-design/vnext-prune-record.md)
records what went and in which order.

## A stance change worth stating out loud

This document used to open with "Evidra is a wire tap, not a gatekeeper… it never blocks
operations." That was true of the old product and is false of this one, and the falsity is
the point of vNext: an evidence record produced by a tool that never refuses anything
records only what an agent chose to do. The endpoint now refuses **protocol-ordering**
violations — a `tools/call` while no operation is open — and refuses nothing about content.

So the accurate sentence is: **Evidra is an execution-evidence recorder that gates the
order of MCP protocol steps, not the semantics of what they do.** It does not classify
commands, score risk, or decide whether an action was wise. §7 allows exactly two modes.

After two probe sets, the sentence that explains *why* the three layers exist is this one:

> **Evidra does not decide whether an agent's claim is true. It preserves the claim beside
> independently observed execution evidence, so that unsupported or surprising claims become
> inspectable.**

That is the reason `declared / observed / reported` stays three separate layers instead of
collapsing into a score: the moment the recorder asserts "the service was not restored", it
has to know the service's state, and it does not. What it does know is that the only things
it watched inside that window were three calls the upstream annotated read-only. Kept side by
side, that disagreement is inspectable by a human; averaged into a number, it is not.

The same boundary applies to §47's in-band report feedback, and it is worth stating precisely
because the mechanism invites the wrong reading: **in-band feedback is post-operation
protocol feedback — not a verifier, and not a precondition for the claim being accepted.** It
arrives after the agent has already written `completed/achieved`, so it cannot correct that
claim; it can only inform the next operation. Evidra's position is unchanged: preserve the
claim, show it beside what was observed, let a reader decide.

## What ships

One idea: an agent working through MCP leaves behind a signed account of what it actually
did **through the wrapped MCP boundary**, which a human can reconcile afterwards without
trusting the agent's own summary. The qualifier belongs in the headline: the recorder sees
tool calls and results, not the state of the world those calls changed, and a reader should
not have to reach §"Scope" twenty lines down to learn how far the account reaches.

```text
agent  ──stdio──▶  evidra-mcp --proxy --evidence-dir DIR -- <upstream MCP server>  ──stdio──▶  upstream
                   │
                   ├─ evidra_prescribe / evidra_report   (two local tools merged into
                   │                                     the upstream's own tool list)
                   └─ events.jsonl + meta.json + keys     (one directory per process)

evidra summarize --dir DIR   ──▶ declared vs observed vs reported, per comparison domain
evidra verify    --dir DIR   ──▶ chain validity, signature validity, coverage, per recorder
```

A closing `evidra_report` is answered in-band with what the proxy observed inside that
operation's window (§47): counts, terminal statuses, tool names, and the read-only
conjunction marked `annotations_verified: false`. It carries no verdict — §35 forbids
rebuilding a human summary inside a tool response — and it is computed from observation,
so it is present whether or not `--evidence-dir` was given.

Three shipped-ish binaries: `cmd/evidra-mcp` (the endpoint — the only thing that enforces
or records), `cmd/evidra` (read side: `summarize`, `verify`, `version` — §41), plus the
experiment surface `cmd/evidra-fixture` (generic MCP conformance upstream for Gate B) and
`cmd/evidra-gatea` (model-arm runner). `ui/` stays in the tree as the asset for a later
relocation; nothing in the vNext path serves or imports it.

## Packages, and why each exists

| Path | Responsibility |
|---|---|
| `pkg/proxy/endpoint*.go` | The merged endpoint: framing both directions, `tools/list` merge, enforcement before forwarding, the §17 event order, upstream lifecycle |
| `pkg/proxy/endpoint_evidence.go` | The store-facing half: prescribe/report/replacement/observation events, argument fingerprints, the §18 store-failure contract |
| `pkg/evidence` | Evidence model v2 (§13–§21): flat envelope, hash-chained append-only file, Ed25519 signatures, JCS + HMAC digests, verification |
| `pkg/report` | Reconciliation (§34, §38): joins declared/observed/reported, derives §34 states and §38 anomalies, emits `summary.json` |
| `cmd/evidra` | Thin read-side CLI |
| `cmd/evidra-fixture`, `cmd/evidra-gatea` | Conformance sensor and experiment runner — not product |

`pkg/evidence` holds exactly one schema. There is no v1 adapter, no cross-process lock, no
`CanonicalAction`: a second write mechanism beside "one directory per recorder process,
one writer" is a bug waiting for a coincidence.

## Invariants the code is built around

1. **Enforcement is protocol-only** (§7). `--enforce=all` refuses an upstream tool call
   while no operation is open; `--enforce=off` observes. Nothing is refused because of what
   an argument looked like, what a tool was named, or an annotation.
2. **Event order is load-bearing** (§17). `execution_started` is appended before the call
   is forwarded; `execution_finished` is appended before the response is relayed. Invert
   the second and an agent that reacts immediately writes `operation_reported` before the
   finished-event describing the execution it reports on. This was caught as a flake, not
   by reading.
3. **Store failure has three distinct contracts** (§18). Cannot append
   `execution_started` → refuse the call with a `recorder_unhealthy` result, forward
   nothing. Cannot append `execution_finished` → still relay the real upstream result,
   degrade coverage, never fabricate a failure. `evidra_report` always answers, but an
   operation is not acknowledged without durable evidence, so it stays open for retry.
4. **Three provenances, no more** (§15): `agent_declared`, `proxy_observed`,
   `recorder_generated`. Derived classes (`unprescribed_execution`) are computed at read
   time and never stored as an event type.
5. **Comparison domains are never averaged** (§20, §24): counts are not comparable across
   enforcement modes, upstreams, or `digest_key_id`s, and `pkg/report` keeps them apart
   structurally rather than trusting the reader's care.
6. **Chain validity, signature validity and coverage are three statements.** A chain can
   verify and still be incomplete; `verify` says both, separately.
7. **Privacy is a bound, not a promise** (§23, §24): argument values leave the process as
   `HMAC(digest_key, JCS(arguments))`; results are hashed up to 4 MiB and marked
   `present | omitted_oversize | unavailable`. Raw observed payloads never enter the
   chain, and the digest key never leaves the recorder directory.
8. **Recording is opt-in per process.** The endpoint has no default evidence path; a
   missing `--evidence-dir` is announced on stderr rather than silently writing into a home
   directory. The read side does honour `EVIDRA_EVIDENCE_DIR`, because a human pointing a
   reader at their usual root is giving an instruction, not being surprised.
9. **The endpoint does not advertise what it cannot cover** (§44): relayed prompts,
   resources and completions are not advertised; `--advertise-passthrough` exists for
   people who have read the boundary statement.
10. **Experiment binary provenance must match the product source revision being evaluated**
    ([§harness](system-design/vnext-experiment-harness.md)): every graded run records the
    source revision and the hashes of the endpoint, fixture and runner it was measured
    against; a cell that mixes builds is an error, not a nuance. The harness produces the
    evidence the product claim rests on, so it is part of the product, not scaffolding.

## Where things are measured

| Gate | Artifact | State |
|---|---|---|
| A — model acceptance (§45, §10) | [`vnext-gate-a-results.md`](system-design/gate-a-results.md) | measured; bars revised from the measurements; enforcement never fired in 80 runs, so the recovery metric is reported, never claimed |
| B — supported-profile fidelity (§46) | [`vnext-gate-b-results.md`](system-design/vnext-gate-b-results.md) | passing on the fixture, with the profile boundary stated and four defects found and fixed |
| C — reconciliation value (§47) | [`vnext-gate-c-reconciliation-probe.md`](system-design/vnext-gate-c-reconciliation-probe.md) | **not passed**: needs a real operational upstream and a reader who did not build the summary |

Each artifact is reproducible from the commands inside it. Recorder directories are not
committed — each holds an ephemeral signing key and a digest key — so `/output/` is
gitignored rather than curated.

## Conventions

Stdlib-first Go; IDs from `github.com/oklog/ulid/v2`; MCP SDK
`github.com/modelcontextprotocol/go-sdk`. `make build test fmt lint tidy` is the loop. The
vNext CI job enumerates every package in the module — no exclusion list survived the
prune — and fails if the package graph drops below eight, so a future mass deletion shows
up as a red build instead of a vacuous green.
