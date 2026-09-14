# Gate B — supported-profile fidelity: measured results

Plan reference: §46 of [vnext-mcp-recorder.md](vnext-mcp-recorder.md). Reproduce
with:

```bash
go test -count=1 -timeout 560s ./pkg/proxy/ ./pkg/evidence/ ./pkg/report/ ./cmd/evidra-mcp/ ./cmd/evidra/
go test -count=1 -race -timeout 560s ./pkg/evidence/... ./pkg/proxy/... ./cmd/evidra-fixture
golangci-lint run ./...
```

A named case can be re-run alone with `go test -run '<TestName>' -v ./pkg/proxy/`.

Upstream under test: `cmd/evidra-fixture`, the generic conformance fixture from §59
step 1. Every case below runs through the real merged endpoint binary
(`evidra-mcp --proxy`), not through an in-process mock: the claim being tested is
about the wrapper, so it has to be measured on the wrapper.

## Conformance checklist (§46)

| Required case | Test | Result |
|---|---|---|
| `initialize` profile, instructions, capability merge | `TestInitializeProfileAndInstructions` | pass |
| `tools/list` = direct tools + local protocol tools | `TestWrappedToolsListEqualsDirectPlusLocal` | pass |
| `tools/list` pagination, local tools on first page only | `TestWrappedToolsListEqualsDirectPlusLocal`, `TestReservedIDNamespaceRejected` | pass |
| `tools/call` to an upstream tool | `TestEnforceOffForwardsUnprescribedCall`, `TestLargeResultPassesThroughAndIsFingerprintedWithinBounds` | pass |
| `tools/list_changed` forwarded and re-merged | `TestToolsListChangedForwardedAndRemerged` | pass |
| progress notifications during a tool call | `TestProgressNotificationsSurviveTheWrapper` | pass |
| cancellation used by tool flow | `TestCancelledExecutionIsRecordedAsCancelled` (both id spellings) | pass |
| request-ID directionality / collision | `TestDirectionIDCollision` | pass |
| >10 MB messages | `TestLargeResultPassesThroughAndIsFingerprintedWithinBounds` (20 MiB) | pass, **after a fix**; see below |
| read / framing errors | `TestClientOversizeFrameRejectedStreamSurvives`, `TestWriteClientPropagatesWriterError`, `TestWriteUpstreamPropagatesWriterError`, `TestWritersFlushExactlyOnceOnSuccess` | pass |
| parallel upstream calls | `TestExecutionsPairByIdUnderParallelCalls` | pass |
| unsupported capabilities advertised down / rejected clearly | `TestInitializeProfileAndInstructions`, `TestUpstreamServerRequestRelayedToClient` | pass |
| reserved-name collision | `TestReservedToolNameCollisionRefusesToStart` | pass |

## Evidence-boundary half of Gate B

| Required property | Test | Result |
|---|---|---|
| hash/signature chain verifies | `TestStoreChainVerifiesAndDetectsTampering`, `TestVerifyRootFindsStoresByNameOfTheirFiles` | pass |
| tamper is detected | `TestStoreChainVerifiesAndDetectsTampering` (rewrites one payload: chain and signature both go invalid) | pass |
| no privacy canary leaks | `TestPrivacyCanaryNeverReachesDiskInPayloadForm` (store), `TestKeysAndMetaAreNotReadableByOthers` (file modes), `TestLargeResultPassesThroughAndIsFingerprintedWithinBounds` (20 MiB body absent from chain) | pass |
| parallel calls do not corrupt event ordering | `TestExecutionsPairByIdUnderParallelCalls` | pass |
| store failure prevents new autonomous work safely | `TestDegradedWindowIsRecordedAndCoverageDegrades` (store), `TestEvidenceStoreFailureStopsForwarding` (endpoint refuses to start on an unusable evidence path) | pass |
| bounded fingerprint policy for large results | `TestResultFingerprintBound`, `TestParseSinceAndGroupKey` (domain keying), `TestLargeResultPassesThroughAndIsFingerprintedWithinBounds` | pass |
| one writer per store structurally, not by convention | `TestRecorderDirsDoNotCollideWithinAMillisecond` (in `pkg/report`), `TestSubstantiveRuleRemovesEmptyStores` | pass, **after a fix**; see below |
| enforcement before forwarding is observable in evidence | `TestEvidenceStoreReproducesEnforcementDecisions` | pass |
| argument fingerprints survive key reordering | `TestJCSIsStableUnderKeyOrderAndFormatting`, `TestJCSNumberAndStringEncoding`, `TestArgumentsFingerprintSurvivesKeyOrder` | pass |

## Findings this gate produced

1. **`--max-message` was decoration; the real ceiling was 64 KiB.** A 20 MiB
   upstream result returned `upstream unavailable: message exceeds 67108864 bytes`.
   `epReadMessage` treated `bufio.ErrBufferFull` — which arrives on every frame
   past the reader's 64 KiB buffer — as proof the message was over the limit, so any
   real server response above 64 KiB became an internal error and a lost execution.
   Fixed by comparing the accumulated frame against the configured maximum and
   draining the remainder of an over-maximum frame before refusing it, so the stream
   stays synchronized. Commit `863d5c8`.

   Worth stating why the experiment did not catch it: Gate A's oversize task accepts
   `completed`, `failed` **or** `cancelled`, because it measures whether an agent
   reports honestly under truncation. A ceiling of that size is invisible to a
   metric about agent behaviour. Only a fidelity gate pointed at the wrapper finds
   it.

2. **Recorder directory names collided within a millisecond.** `NewRecorderDirName`
   sliced the front of a ULID, which is its timestamp component, so two recorders
   started in the same millisecond got the same path; the second process resumed and
   appended into the first one's file. The per-process single-writer layout of §20
   was nominal, not structural. Fixed with the random tail plus `os.Mkdir` and a
   redraw on collision. Commit `a6eb3ef`.

3. **`evidence.VerifyRoot` filtered stores by directory name**, so an aggregation
   folder of renamed or copied stores was reported as "no recorder directories" —
   an answer that reads as "nothing was recorded". Stores are now identified by
   `events.jsonl` presence. Commit `6192def` (with `863d5c8` follow-up tests).

4. **`--since` did not narrow verify counts.** `ReadStore` honoured the window while
   `VerifyStore` counted the whole file. Verification remains full-history (a chain
   cannot be trusted by sampling) and the counts are windowed; an empty window now
   exits non-zero instead of reporting a clean bill of health for nothing.

## Fidelity claim, as measured

> **Transparent for the supported MCP tool profile**, with one known boundary: a
> single frame above `--max-message` (default 64 MiB) in either direction is
> refused and the stream survives; nothing larger has been attempted.

## Not covered here

- No third-party MCP server has been wrapped: the fixture is authored to the
  supported profile and cannot exhibit server-specific deviations (unusual
  `_meta`, non-standard content types, HTTP transport quirks). §47 requires one
  real operational server for that reason.
- HTTP/SSE transports, multi-upstream multiplexing, and generic gateway behaviour are
  out of scope by §44 and are not claimed.

## Amended by §43 step 5 (relay deletion)

The relay's framing tests (`TestWriteLine_*`, `TestWriteJSONLine_*`, the mutation-classifier
tests) were deleted with the relay they covered. The endpoint's own write paths are now
covered in `endpoint_write_test.go`, which found a real asymmetry on the way in: a write
error surfaced at `Flush()` came back bare, while one caught at `Write()` was labelled
with its direction — so a client-visible failure and an upstream-visible failure were not
distinguishable in the log. Both are wrapped now.
