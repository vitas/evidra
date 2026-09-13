# Changelog

## Unreleased

### vNext experiment — step 3a: protocol enforcement (branch `vnext/mcp-recorder`)

- `--enforce=all` (default) implements the single rule of §7: with no open operation an upstream `tools/call` is not forwarded, is noted as a `protocol_violation`, and the agent gets a tool result — not a transport error — containing `no_open_operation`, the tool it tried, and the instruction to prescribe and retry. Declared read-only tools are blocked by the same rule; annotations stay reporting data.
- `--enforce=off` forwards everything and never blocks, so prescription coverage in that mode measures voluntary behavior. An unsupported mode (`--enforce=mutations`) fails at startup listing the two valid values.
- Enforcement resumes the moment an operation closes: a report returns the session to blocked state, tested through the fixture's own restart counter so a blocked call is proven never to have reached the upstream.
- `cmd/evidra-mcp` wrapping flags moved into `registerProxyFlags`/`dispatch` to keep `run()` inside the lint budget, and they hold `flag` pointers rather than copies.
- 3 new endpoint tests (blocked-and-proven, observe-only, unknown mode); 14 endpoint tests green with `-race`.
### vNext experiment — step 2 (branch `vnext/mcp-recorder`)

- `evidra-mcp --proxy -- <server>` is now the vNext merged endpoint: one upstream child process plus Evidra's own `evidra_prescribe` / `evidra_report`, appearing to the agent as a single MCP server. The pre-vNext relay stayed reachable as `--legacy-proxy` until the §59 step-10 prune.
- Tool-list composition follows §27: local tools appear on the first upstream page only, upstream cursors and per-tool JSON survive byte-identically, and `notifications/tools/list_changed` is forwarded and re-merged. §6 is enforced at startup — an upstream advertising `evidra_prescribe` or `evidra_report` makes the endpoint refuse to start rather than rename anything (tested through a new `--steal-names` fixture flag).
- `initialize` is composed rather than echoed: client-facing capabilities are the supported profile with untested families advertised down (`--advertise-passthrough` opts them back in), protocol version is negotiated explicitly, and upstream instructions are appended behind a source boundary instead of being overwritten.
- Direction-safe bookkeeping per §28: requests, notifications and responses are classified by method+id shape, per-direction pending tables make a client request id and an upstream server-to-client request id collide harmlessly, and pending client requests are answered with an explicit error when the upstream dies instead of hanging. Framing rejects oversized frames with the session intact.
- §11 and §32 behavior is live in memory: one open operation per process, `operation_already_open` with `continue_current` / `abandon_and_replace`, `operation_id_mismatch`, `no_open_operation`, and an idempotent `already_reported` for a duplicate close. Durable evidence lands with the v2 store in step 4 behind a narrow recorder seam.
- 10 endpoint conformance tests in `pkg/proxy/endpoint_test.go` drive the real fixture and the real CLI as child processes, including the direct-vs-wrapped list equivalence that is step 2's exit criterion.
### vNext experiment — §43 prune: written as a plan, deliberately not executed

- `docs/system-design/vnext-prune-plan.md` is the deletion-ready inventory for §43:
  every removal item mapped to the packages and files that implement it, the vNext
  surface that must survive, and a five-step order that keeps CI green at each commit
  (leaves first, then the analysis chain, then `pkg/mcpserver`, then the legacy relay
  together with its tests, then v1 evidence shapes), each step gated on build/test/race
  plus a Gate A re-grade.
- It is not executed, because §43 conditions deletion on Gates A–C being green and
  none of them is: Gate A's strong arm is 11/15 against a 15/16 bar with zero blocks
  in 80 runs, Gate C is explicitly not passed. Deleting the measured legacy path while
  the replacement's central question is still open is the failure §43 warns about,
  so the artifact states the precondition rather than satisfying the checklist item.
- The doc also records the two ways forward: clear the bars as written, or revise them
  to what the cheap arms demonstrably do — with the revision landing in the same commit
  as the first deletion.

### vNext experiment — Gate A counts are now read from the recorder, not inferred

- `cmd/evidra-gatea` reconciles each finished run against its own store
  (`<run>/evidence/recorder-*/events.jsonl`): `blocked_attempts`,
  `unprescribed_executions`, per-operation prescribe/report counts, chain and
  signature validity, and coverage come from signed events, and `counts_from` in
  every `result.json` says which account produced them (`store` or `transcript`).
  The transcript path stays as the fallback for runs that asked for no store, rather
  than being quietly relabelled as evidence.
- An execution recorded with no open operation under `--enforce=all` is now a
  per-run failure (`enforcement hole: …`), not a cell statistic: a number in that
  field means the recorder watched a call get through that it existed to refuse, and
  a rolled-up average is the wrong place to discover it.
- An invalid chain or signature invalidates the run's claim to be evidence at all, so
  it is recorded as a failure of that run instead of being averaged into a cell.
- `readStoreFacts` refuses a run directory holding two recorder stores rather than
  picking one: two writers in one run means the runner's isolation of runs broke, and
  choosing one arbitrarily would hide it.
- Verified against the existing free-arm probe set with `--regrade` (no model calls
  re-run): all 8 runs now report `counts_from: store` with the enforcement counts
  unchanged, which is the outcome hoped for — the recorder agrees with what the
  runner inferred from traffic.

### vNext experiment — Gate B conformance, and the two gates' first artifacts

- Gate B's ">10 MB messages" line exposed the round's most consequential defect:
  `--max-message` was never enforced, because `epReadMessage` treated
  `bufio.ErrBufferFull` (raised on every frame past the reader's 64 KiB buffer) as
  proof of an over-long message. Any upstream response above 64 KiB became
  `upstream unavailable` plus a lost execution. Fixed by bounding the accumulated
  frame and draining an over-limit frame before refusing it. Gate A did not see this
  because its oversize task accepts an honest failure, and a 64 KiB ceiling is
  invisible to a metric about agent behaviour.
- Two conformance tests added with it: progress notifications must survive the
  wrapper (id-less notifications are dropped by a relay that tracks only
  request/response pairs), and a 20 MiB result must cross intact while its evidence
  record stays bounded to `omitted_oversize` with no key material in the chain.
- `VerifyRoot` identified stores by directory-name prefix, so a renamed or aggregated
  folder of valid stores was reported as "no recorder directories" — an answer that
  reads as "nothing was recorded". Stores are now identified by their files.
- `docs/system-design/vnext-gate-b-results.md`: §46's checklist mapped to named tests,
  measured pass/fail, the four findings above, the fidelity claim as measured, and
  the boundary that no third-party server has been wrapped.
- `docs/system-design/vnext-gate-c-reconciliation-probe.md`: 8 free-arm sessions run
  through the new store and reconciled, with the reproduction commands and an
  explicit **not-passed** verdict — no real operational server, no human judgement on
  record, and no session yet where declarations and observations genuinely diverge.
  It records what reconciliation did add (per-operation observation counts from
  signed evidence, chain validity separate from coverage, empty windows reported as
  empty) and the cheapest path to a real verdict, led by §47's still-unimplemented
  in-band `evidra_report` feedback.
- `/output/` is gitignored on purpose: every recorder directory holds an ephemeral
  signing key and a digest key that must not enter history. Verified for the probe
  set that no substring of either key appears in `summary.json`.

### vNext experiment — step 9: `evidra summarize` and `evidra verify`

- Two vNext commands, deliberately thin over `pkg/evidence` and `pkg/report` so an
  agent, a script and a human at a terminal cannot grow three definitions of the same
  metric. `version` was already there; §59 step 9 asks for nothing more.
- `summarize --dir <root> [--since 7d]` prints the terminal summary and writes
  `summary.json` beside the evidence. `verify --dir <root> [--since ...]` reports
  `chain`, `signature` and `coverage` as three statements per recorder store, with the
  enforcement mode, upstream and `digest_key_id` on every line — a number without its
  comparison domain is not a fact.
- `--since` turned out to be unimplemented in the verify path: the window filter existed
  in `ReadStore` while `VerifyStore` counted the whole file, so a query for "last 7
  days" reported every record. Verification is now explicitly full-history (a chain is
  only as trustworthy as all of it) and the *counts* are windowed, which is what a
  reader is asking for.
- An empty window is a failure, not a pass. `verify` on a store whose records all fall
  outside the window exits non-zero with "none with records in the requested window":
  silence from a query that examined nothing must not be readable as a clean bill of
  health.
- Tests: 6 CLI tests (summary JSON round trip plus anomaly visibility in the terminal
  view, chain/coverage separation, mode label presence, byte-level tamper detection
  through the CLI, missing-directory usage, since-window behaviour) and a report test
  that a window narrows records and operations without invalidating the chain.

### vNext experiment — step 8: reconciliation, and two structural bugs it exposed

- `pkg/report` implements §34/§38: it reads every recorder store under an evidence
  root, joins prescriptions with the executions carrying their `operation_id` and
  with the agent's terminal report, and emits a `summary.json` that keeps each
  comparison domain separate (`enforce mode | upstream | digest_key_id`). Derived
  states are exactly the four the plan names — `reported`, `replaced_without_report`,
  `interrupted_session`, `lifecycle_unknown` — and no report is ever synthesized for
  an operation that has none.
- §38.A's anomaly (`CLAIMED_ACHIEVED_WITHOUT_OBSERVED_EXECUTION_IN_SCOPE`) is emitted
  as an evidence anomaly with its observation scope, never as fraud. §38.B's
  same-fingerprint recovery reports `insufficient_fingerprint_data` instead of
  "no later success" whenever the comparison could not actually be made, which is the
  difference between a measurement and an accusation. §38.C's read-only fact carries
  the disclosure that server annotations are unverified.
- `blocked_attempts_for_operation` turned out to be unimplementable as named: a block
  only happens when no operation is open, so an operation-scoped block count is
  structurally always zero, and printing that would hide the exact distinction §38
  wants (nothing attempted versus five refusals). The field is
  `blocked_attempts_in_session`, the narrowest scope that can exist.
- `first_attempt_protocol_compliance` is filled only in `enforce=all` and
  `voluntary_prescription_coverage` only in `enforce=off`, so no number can be read
  as spanning both.
- Two bugs found while building it, both structural:
  - `NewRecorderDirName` sliced the **front** of a ULID, which is its timestamp
    component: every recorder started in the same millisecond got the same directory
    name, and the second process appended into the first one's file. Single-writer
    per store was not holding. Now the random tail plus `os.Mkdir` with redraw, so a
    name collision cannot be silently resumed into.
  - The execution index deleted entries as they paired, so a fully reconciled session
    assembled to zero executions. Starts now live in two indexes: one open-set that
    shrinks, one complete record that operations are built from.
- Tests: 5 reconciliation tests (anomaly plus block attribution, mode separation,
  interrupted versus lifecycle-unknown, fingerprint metrics including the
  insufficient-data branch, recorder-name uniqueness) and they assert the negative
  too — a cell where prescription was required must not report voluntary coverage.

### vNext experiment — step 6: execution pairing, cancellation, annotations

- `notifications/cancelled` now reaches the outstanding call it names. Request ids
  are matched by value: the forwarded frame was keyed as the client spelled it, and
  the cancel echoes the id back, so a numeric `7` and a string `"7"` both resolve.
  Before this the cancel landed nowhere, which only showed up as an execution that
  started and never finished - a recorder lying by omission.
- A session that ends with calls in flight closes those executions itself, while
  the store is still open, instead of racing the child's EOF. Cancelled-and-never-
  answered is recorded as `cancelled`; started-and-abandoned is `unknown`. A call
  that answered anyway after being cancelled is recorded as answered: the terminal
  event states what happened inside the boundary, not what the client hoped.
- Terminal events are appended **before** the response is relayed, per §17. A test
  that asserted the event order caught the inversion as a flake: relaying first let
  an agent that reacts to a result immediately write its `operation_reported` ahead
  of the `execution_finished` it describes, and no downstream reconciliation can
  unscramble two events whose relative order is the fact being measured. The real
  upstream result is still relayed when the append fails (§18).
- Tests: parallel `slow` calls pair by `execution_id` without crossing their
  durations; `arguments_hmac` is stable across key reordering through the real
  endpoint (not just the helper) while a changed value moves it; and cancellation
  is asserted for both request-id spellings.

### vNext experiment — step 4: evidence.v2 store wired into the endpoint

- `pkg/evidence` gains the v2 model from plan §14-§21: a flat `evidra.evidence.v2`
  envelope (`seq`, `event_id`, `recorder_instance_id`, `session_id`, `operation_id`,
  `upstream_id`, `provenance`, `previous_hash`, `hash`, `signature`), the ten event
  types of §16 including `recorder_degraded`, and start/finish execution pairing by
  `execution_id`. `unprescribed_execution` stays a derived class, not an event type.
- RFC 8785 JCS canonicalization is implemented in-package (UTF-16 code-unit key
  ordering, ECMAScript `NumberToString`, JSON.stringify's escape set) so argument
  fingerprints survive key reordering without adding a dependency.
- Fingerprints per §23-§24: per-store HMAC-SHA256 key at 0600 with a non-secret
  `digest_key_id` as the comparison domain; result fingerprints bounded at 4 MiB
  with explicit `present` / `omitted_oversize` / `unavailable` states; error messages
  keyed rather than stored.
- Store layout per §20: one directory per recorder process
  (`recorder-YYYYMMDD-HHMMSS-<id>/events.jsonl|meta.json|signing.key|digest.key`),
  which makes a cross-process chain race structurally impossible instead of
  locked-around. §19's serialized append takes a builder that receives
  `(previous_hash, seq)`, and every record is flushed: a buffered recorder loses
  exactly the tail that an interrupted execution needs.
- §18 failure behavior is implemented, not deferred: a call whose
  `execution_started` cannot be written is refused with a `recorder_unhealthy`
  tool result; a lost `execution_finished` keeps the real upstream result intact,
  degrades coverage, and never fabricates an operational failure; `evidra_report`
  stays callable and is not acknowledged without durable evidence (the operation
  stays open so the agent can retry). A recorder that saw nothing removes its own
  directory.
- `VerifyStore` / `VerifyRoot` answer chain validity, signature validity and
  evidence coverage as three separate statements, with `--since` accepted as `7d`
  or an RFC 3339 instant, and grouping by enforcement mode + upstream +
  `digest_key_id` so a summary cannot average `enforce=all` with `enforce=off`.
- The merged endpoint writes this store when `--evidence-dir` is given. No
  directory, no store — but stated on stderr rather than silent, because
  "enforcement happened" and "evidence was kept" are different claims.
- Tests: 8 for the store (chain round trip, tamper detection after rewriting a
  record, degraded window with valid chain but incomplete coverage,
  lifecycle-only directory removal, key permissions, canary, fingerprint bound,
  `--since`/group key) and 3 end-to-end through the real endpoint: a refused
  unprescribed call appears as `protocol_violation` and leaves **no** execution
  record — absence is the proof forwarding was gated — plus annotations recorded
  with their absence preserved for an unannotated tool, and a refused startup when
  the evidence path is not a directory.

### vNext experiment — step 3b: Gate A runner, and the first pilot

- `cmd/evidra-gatea` drives the merged endpoint with an OpenAI-compatible
  tool-calling client: one `evidra-mcp --proxy --enforce=<mode> -- <fixture>`
  process per run, per-run JSONL transcript (frames, completions, tool events,
  verdict), `summary.json` with per-cell metrics, §10's gate conditions and the
  `git` revision under test.
- §10's failure classes became `invalid_run` and left every denominator: gateway
  credit/balance, model access, transport, wall-clock timeout,
  `finish_reason=length`. A preflight probe per arm is mandatory before the first
  task, and `--calibrate` measures the prompt-token cost of the two protocol tool
  definitions separately from per-run traffic (94 tokens on qwen3.8-flash).
- `--dry-run` replays a scripted agent (`compliant|late|noprescribe|noreport`) so
  predicates, metrics and artifacts are testable at zero token cost;
  `TestEveryTaskPredicateIsSatisfiable` fails if any task cannot be passed by a
  protocol-correct sequence.
- Task 7 (`change-of-mind`) was redesigned after the first 32-run pilot demanded
  `abandon_and_replace` specifically and rejected a model that closed the dropped
  operation honestly as `abandoned`. The predicate now accepts both §11 endings
  and rejects only the case that matters: an `achieved` record for dropped work.
- Protocol rules are no longer paraphrased in the runner's prompt (they are
  relayed verbatim from `initialize.instructions`), open operations are no longer
  nagged about, and agent-visible tool results are capped at 8 KiB with an
  explicit marker. All three were inflating or distorting a Gate A metric.
- First free-arm pilot (qwen3.8-flash, 32 runs, 0 invalid): `enforce=all` 12/16
  task success with 16/16 first-attempt compliance and zero blocked attempts;
  `enforce=off` 16/16 voluntary coverage with zero unprescribed executions.
  Recovery after first block stays `not_measurable` — nothing was ever blocked, so
  this arm provides no evidence for it yet.

### Gate A arms pinned from live endpoint probes (plan §10)

- §10 now names its arms instead of describing them: `qwen3.8-flash` @ `api.b.ai/v1` (free at time of writing), `deepseek-flash` @ `api.deepseek.com/v1` (the deployment the harness config displays as DeepSeek-V4.1-Flash), `deepseek-v4-pro` @ `api.deepseek.com/v1` as the ceiling arm. 80 runs total, ordered cheapest-first so a protocol redesign never spends metered tokens on discarded runs. Two of three arms share an endpoint, which removed the need for a transport-control cell.
- Added a reasoning-token budget rule: all three arms emit `reasoning_content`, and `max_tokens: 80` truncated a valid tool call into `finish_reason=length`. Budget is >= 2048 per turn and truncation is harness fault. Protocol overhead is now reported as visible (tool definitions + prescribe/report traffic) and total (including reasoning), never merged.
- Added `invalid_run` classification (gateway balance/access errors, persisting 429, transport timeout, `finish_reason=length`, preflight model-id mismatch) plus a mandatory preflight probe per arm, so account state cannot be read as agent behavior.
- Pinning extended to provider + base URL + verbatim API model id + display name + `cost_basis: free|metered`, because one product is reachable under three names across two gateways and no claim may silently depend on a promo price.
- Stale "64 runs" references corrected in §45, §59 step 3, and the thresholds heading.

### vNext experiment — step 1 (branch `vnext/mcp-recorder`)

- Added `docs/system-design/vnext-mcp-recorder.md`: the vNext execution-evidence plan (v8) as a tracked document — table of contents, 60 sections, plus the one-page implementation appendix (§59) that implementation agents work from.
- Added `cmd/evidra-fixture`: the deterministic MCP fixture server required by plan §30. Implemented directly against JSON-RPC over stdio rather than on the go-sdk, because SDK v1.5.0 cannot express the annotation states the plan has to distinguish (`readOnlyHint: false` vs no `annotations` key at all vs a contradictory set — `ToolAnnotations.ReadOnlyHint` is a plain `bool` with `omitempty`). The fixture exercises cursor pagination, >10 MiB results, server→client requests, `notifications/cancelled` with no response, `notifications/progress` via `_meta.progressToken`, `notifications/tools/list_changed` with a tool that really appears, request IDs that collide across directions, and oversized-message rejection that keeps the stream alive. `--serial` yields byte-stable response order for golden summaries; the default handles requests concurrently so parallel-call pairing can be tested at all.
- Added `.github/workflows/ci-vnext.yml`: the reduced supported build graph of plan §4 — 31 core packages certified, hosted platform (`cmd/evidra-api`, `internal/{api,apiutil,analytics,analyticsdb,analyticsvc,assessment,auth,automationevent,config,db,gitops,ingest,sarif,store}`, `pkg/{client,mode}`) and the tag-gated legacy e2e suite excluded. `ci.yml` is untouched: `main` and the public release channel keep the full gate set.
- `internal/signal/unprescribed_mutation_test.go` is gofmt-clean again; it was the one file failing the formatter gate that this branch inherits from `main`.

### Prompt contract v1.4.0
- New published contract `v1.4.0` (source tree `prompts/source/contracts/v1.4.0`, generated bundles refreshed): agent guidance now states claim linking — a prior `prescribe_smart`/`prescribe_full` is auto-linked to the executed `run_command` mutation, unlinked mutations are recorded `auto_prescribed` and counted as `unprescribed_mutation`. Replaced the "skip explicit prescribe for run_command" guidance with a prescribe-first incentive; behavioral signal list extended to nine.
- `scripts/prompts-generate.sh` / `prompts-verify.sh` defaults bumped to v1.4.0; embedded `DefaultContractVersion`/`DefaultContractSkillVersion` now `v1.4.0`/`1.4.0`.

### Unprescribed mutation signal + prescription claiming
- `run_command` mutations now link to a prior model-issued prescription when one claims the same normalized action (tool + operation + resource, within prescription TTL); only unlinked mutations write an auto-derived prescription flagged `auto_prescribed: true` in the payload.
- New behavioral signal `unprescribed_mutation` (9th detector): counts mutations executed with no prior model claim — protocol compliance becomes measured data instead of a silent gap in evidence. Weighted `0.0` in the default profile pending calibration on real agent traffic.
- Additive `PrescriptionPayload.auto_prescribed` field; older readers and validators unaffected (hash format unchanged).
- MCP server-derived prescriptions now record actor origin `evidra:auto`.

### External Evidence Bundle v1
- Added `evidence.EphemeralSigner`: public per-run Ed25519 signer for external bundle producers (`pkg/evidence/signer_ephemeral.go`).
- Added `evidence.BundleManifest` with `bundle.json` sidecar read/write helpers and `evidence.ValidateBundle` (`pkg/evidence/bundle.go`).
- `evidra validate` now auto-detects `bundle.json` and verifies signatures against the embedded public key.
- Added `docs/external-evidence-bundle-v1.md` — stable producer spec for third-party evidence stores.
- Added conformance fixture `tests/external_bundle_v1/` (generated via public API only) with chain + tamper-detection tests.

## v0.5.30 — 2026-06-17

### SEO
- Restored the Bench-first root SEO title and description contract for `evidra.cc`.

## v0.5.29 — 2026-06-16

### SEO
- Added canonical metadata, Open Graph image, structured data, favicon, and no-JS crawlable content for `evidra.cc`.
- Stopped serving the SPA shell for missing asset URLs such as `/favicon.ico`.

## v0.5.28 — 2026-05-21

### SEO
- Added root sitemap and robots files for search engine crawling.

## v0.5.27 — 2026-05-21

### Site Verification
- Added the Google Search Console verification file to the embedded web UI.

### Landing Page
- Added the open source Evidra Bench repository link to the landing page.

## v0.5.26 — 2026-05-08

## v0.5.25 — 2026-05-08

## v0.5.24 — 2026-05-08

## v0.5.23 — 2026-05-08

## v0.5.22 — 2026-05-06

## v0.5.21 — 2026-05-06

### Repository Scope
- Removed the hosted bench API, runner control plane, and embedded `/bench` UI from the core Evidra API; Evidra Bench now lives as a separate product surface at `https://bench.evidra.cc`.

## v0.5.20 — 2026-04-13

## v0.5.19 — 2026-04-13

## v0.5.18 — 2026-03-28

### Bench Hosted Execution
- Added first-class hosted `execution_mode` support to `POST /v1/bench/trigger`, with optional `provider|a2a` selection and default `provider`
- Threaded `execution_mode` through trigger state, runner job persistence, runner claim payloads, OpenAPI, and bench dashboard controls
- Mapped hosted `execution_mode=a2a` to the bench executor's internal `config.adapter=a2a` for direct benchmark execution

### Bench Model Configuration
- `GET /v1/bench/models` — list tenant-visible models with `available` field based on platform env var presence
- `PUT /v1/bench/models/{model_id}/provider` and `DELETE` — tenant provider overrides (disabled until encryption is implemented)
- `PUT /v1/admin/bench/models/{model_id}` — invite-gated platform route for global model defaults
- Trigger handler auto-resolves provider from model catalog when not supplied in request
- Trigger handler validates API key is configured before accepting jobs
- Input validation: upsert requires at least one non-empty field; delete returns 404 for nonexistent providers

### MCP Deferred Tool Loading
- Deferred tool schema registry — `prescribe_smart` and `report` schemas loaded on demand, not at initialize
- `describe_tool` meta-tool for clients to fetch individual tool schemas
- `run_command` path preferred in initialize instructions
- MCP contract bumped to v1.3.0


## v0.5.13 

### Contract v1.2.0
- Contract extended with DevOps operations (run_command, write_file, diagnosis protocol, safety rules)
- MCP prompt templates (prescribe-smart, prescribe-full, diagnosis) generated from contract source
- All prompts generated from `CONTRACT.yaml` — no hardcoded files
- SKILL.md leads with DevOps operations, protocol compressed to essentials

### Skill Rework
- SKILL.md rewritten: DevOps ops first (~600 tokens), protocol second (was ~2000)
- Trigger description updated to include all DevOps ops (read + write)
- MCP prompts: prescribe-smart, prescribe-full, diagnosis as on-demand resources

### Bench Intelligence Endpoints
- `GET /v1/bench/signals` — aggregated signal counts (protocol_violation, retry_loop, blast_radius) from run scorecards
- `GET /v1/bench/regressions` — detects scenario/model pairs where the latest run failed but previous runs passed
- `GET /v1/bench/insights?scenario=X` — failure analysis with check failure stats, model breakdown, behavior metrics (pass vs fail avg turns/tokens/cost)
- `GET /v1/bench/compare/models` — fixed to accept `?models=X,Y,Z&scenarios=A,B` for multi-model matrix comparison (in addition to legacy `?a=X&b=Y` pairwise)
- `POST /v1/bench/scenarios/sync` — upsert scenario metadata from bench CLI

### MCP Modes And Ingest
- Split the MCP lifecycle surface into `prescribe_full` and `prescribe_smart`, with clearer public mode wording around Full Prescribe, Smart Prescribe, and Proxy Observed
- Added authenticated external lifecycle ingest routes for adapter-driven `prescribe` and `report` creation, and routed webhook ingestion through the same shared service
- Extended payload taxonomy so entries now carry execution flavor plus explicit `evidence.kind` and `source.system`


## v0.4.11

### GitOps And Argo CD
- Added controller-first Argo CD integration for self-hosted `evidra-api`, with zero-touch reconciliation capture and explicit `evidra.cc/*` correlation
- Kept GitOps evidence on the standard `prescribe` / `report` lifecycle using `payload.flavor = reconcile`
- Added shared automation event emission for mapped Argo CD webhook and controller-reported lifecycle entries
- Split execution flavor from ingest taxonomy so payloads can also record `evidence.kind` and `source.system`, and renamed `pipeline_stage` to `workflow`
- Refactored `evidra-api` startup initialization to reduce complexity without changing startup behavior

## v0.4.10 

### Benchmark API
- Added benchmark table to landing page UI
- Added input validation for benchmark run suite field
- Capped benchmark list query limit to 100

### Hosted Analytics And API Contracts
- Replayed stored evidence chronologically in hosted scorecard/explain so self-hosted analytics matches CLI/local signal behavior for order-sensitive detectors
- Added required `operation_id` to generic webhook events and used it as the stable prescribe/report lifecycle correlation key
- Moved API key issuance quota checks behind invite-secret validation so rejected onboarding attempts do not burn shared rate-limit budget
- Replaced fire-and-forget `last_used_at` writes on API key lookup with bounded inline updates
- Restored a fixed eight-signal public scorecard contract instead of auto-expanding API output to every registered signal

### MCP
- Fixed the `get_event` MCP tool output contract so stored report events can be returned without structured-output schema validation failure
- Added explicit MCP output schema coverage for `get_event` payload shapes

### MCP And Prompts
- Updated MCP report schema, tool descriptions, prompt contracts, and generated prompt artifacts for explicit verdicts and declined decisions
- MCP now records not only actions but also deliberate refusals with rationale

## v0.4.2

### Signals
- New signal: `risk_escalation` — detects when an actor's operations exceed their baseline risk level (8th signal, weight 0.10)
- Baseline computed as mode of actor+tool risk levels in 30-day rolling window
- Demotions tracked internally as `risk_demotion` sub-signal (informational, no penalty)
- Signal Spec updated to v1.1

### Telemetry
- `risk_escalation` added to allowed signal names in OTLP metrics export

### Documentation
- [MCP Setup Guide](docs/guides/mcp-setup.md) — install, connect agents (Claude Code, Cursor, Codex, Gemini CLI, OpenClaw), configuration, troubleshooting
- MCP Setup section added to landing page with editor-specific config snippets
- Signal 8 definition added to EVIDRA_SIGNAL_SPEC.md
- All "7 signals" references updated to 8 across docs, UI, and OpenAPI spec
- Architecture overview moved to `docs/ARCHITECTURE.md`

### Testing
- E2e test: staging→production escalation through full CLI pipeline
- Score stability regression test (zero-count risk_escalation does not affect score)

## v0.3.1 

### CLI
- `evidra run` — execute commands live and record lifecycle outcome (prescribe + execute + report in one call)
- `evidra record` — ingest completed operations from structured JSON input
- `evidra keygen` — generate Ed25519 signing keypair
- Assessment output includes `score`, `score_band`, `basis` (preview vs sufficient), and `signal_summary`
- `--canonical-action` flag for pre-canonicalized actions (Pulumi, Ansible, CDK escape hatch)
- Kustomize support added to K8s adapter (`--tool kustomize`)

### Observability
- OTLP/HTTP metrics export: `evidra.operation.signal.count` and `evidra.operation.duration_ms`
- Bounded-cardinality labels: tool, environment, result_class, signal_name, score_band, assessment_mode
- Configuration via `EVIDRA_METRICS_TRANSPORT`, `EVIDRA_METRICS_OTLP_ENDPOINT`, `EVIDRA_METRICS_TIMEOUT`
- [Observability Quickstart](docs/guides/observability-quickstart.md) with collector setup and PromQL examples

### Protocol
- Session ID auto-generated at ingress when omitted
- `operation_id` and `attempt` fields on evidence entries
- `session_start`, `session_end`, `annotation` entry types
- Signing enforced on every evidence entry (strict mode default)
- Trace defaults: `trace_id` defaults to `session_id`, optional `span_id`/`parent_span_id`
- Evidence write mode: `strict` (default) or `best_effort`

### Canonicalization
- Docker adapter: docker, nerdctl, podman, compose
- OpenShift resources: DeploymentConfig, Route, BuildConfig, ImageStream
- Noise filtering: managedFields, uid, resourceVersion, creationTimestamp, last-applied-configuration

### Documentation
- [Supported Tools](docs/SUPPORTED_TOOLS.md) reference with adapter matrix and risk detectors
- [Observability Quickstart](docs/guides/observability-quickstart.md) — OTLP setup, Grafana/Prometheus queries, CI examples
- [Terraform CI Quickstart](docs/guides/terraform-ci-quickstart.md)
- [Scanner SARIF Quickstart](docs/integrations/SCANNER_SARIF_QUICKSTART.md) rewritten with run/record patterns
- [CLI Reference](docs/integrations/cli-reference.md) — unified command reference
- [Setup Evidra Action](docs/guides/setup-evidra-action.md) — GitHub Actions + generic CI install

### Testing
- Real-world e2e test suite: K8s, Terraform, Helm (Redis, ingress-nginx), ArgoCD, Kustomize, OpenShift
- E2e tests verify actual canonicalization output (resource_count, resource_identity, risk_tags, noise immunity)
- Run/record parity contract tests
- MCP schema-struct parity contract test
- Signal validation scenarios in CI

### CI/CD
- E2e tests gate release pipeline (release-guard → test → e2e → snapshot + docker → goreleaser)
- Homebrew tap publishing via GoReleaser
- Docker image: `ghcr.io/vitas/evidra-mcp`
- `setup-evidra` GitHub Action for CI adoption

### Fixes
- Evidence chain: in-process ID cache for faster entry lookup
- Findings correlation: correct TraceID, attach SessionID/OperationID/Attempt
- Lifecycle flows unified with session invariant enforcement
- Removed dead code (MaxBaseSeverity, RehashEntry, SegmentFiles)

## v0.3.0

First public release of Evidra Benchmark.

### Core Pipeline
- Canonicalization adapters: Kubernetes (kubectl, oc, helm), Terraform, Docker (docker, nerdctl, podman), generic fallback
- Risk matrix (operation_class x scope_class) with 7 catastrophic detectors
- Eight behavioral signals: protocol violation, artifact drift, retry loop, blast radius, new scope, repair loop, thrashing, risk escalation
- Weighted reliability scoring with safety floors and band classification
- Ed25519 evidence signing with strict/optional modes and key generation

### CLI (`evidra`)
- `prescribe` — record intent before infrastructure operations
- `report` — record outcome after execution
- `scorecard` — compute reliability score from evidence chain
- `explain` — detailed signal breakdown with sub-signals
- `compare` — side-by-side actor comparison with workload overlap
- `--scanner-report` flag for SARIF ingestion (Trivy, Kubescape)
- `--canonical-action` flag for pre-canonicalized actions
- Tool and scope filtering on scorecard/explain/compare
- `run` — execute command live and record lifecycle outcome
- `record` — ingest completed operation from structured JSON input
- `validate` — verify evidence chain integrity and signatures
- `ingest-findings` — ingest SARIF scanner findings as evidence entries
- `keygen` — generate Ed25519 signing keypair

### MCP Server (`evidra-mcp`)
- Stdio transport for MCP-based automation integration (including AI agents)
- Tools: prescribe, report, get_event
- Session/trace/span correlation fields for multi-step workflows
- Optional retry loop tracking

### Protocol (v1.0 Foundation)
- Session/run boundary hardened: persisted evidence entries always include `session_id` (generated at ingress when omitted by caller)
- Correlation defaults documented: `trace_id` defaults to `session_id`, with optional `span_id` and `parent_span_id`
- Actor identity: `actor.instance_id` and `actor.version` (optional, not used in metrics)
- Scope dimensions: `scope_dimensions` map for detailed environment metadata (cluster, namespace, account, region)
- Protocol spec: `docs/system-design/EVIDRA_PROTOCOL.md`

### Evidence Chain
- Append-only JSONL with hash-linked entries
- Segmented storage with automatic rotation (5MB default)
- File-based locking for concurrent access

### Build
- Go 1.23 minimum (CI pinned from `go.mod`)
- Cross-platform binaries via GoReleaser (linux/darwin/windows, amd64/arm64)
- Homebrew: `brew install samebits/tap/evidra-mcp`
- Docker: `ghcr.io/vitas/evidra-mcp:0.3.0`

### Known Limitations
- ArgoCD uses generic adapter (no Argo-specific metadata)
- MinOperations=100 required for scoring (low-volume actors get `insufficient_data`)
- Optional signing mode (`EVIDRA_SIGNING_MODE=optional`) uses ephemeral keys and is not durable across restarts
- No centralized API server (v0.5.0)
