# Evidra Canonicalization Test Strategy

## Status
Simplified testing contract. Replaces the previous 8000-test proposal.

## Principle
One test type per guarantee. No test infrastructure that costs
more to maintain than the code it tests.

---

## 1. Two Tests. That's It.

### Test A: Golden Corpus (10 cases total)

Each case has an input file and a digest file. 3-5 key cases also
have a full canonical_action snapshot to catch regressions in
resource_count, scope_class, operation_class, and risk_tags that
a digest-only check would miss.

```
tests/golden/
  k8s_deployment.yaml           → k8s_deployment_digest.txt
  k8s_multidoc.yaml             → k8s_multidoc_digest.txt
                                  k8s_multidoc_action.json     ← snapshot
  k8s_privileged.yaml           → k8s_privileged_digest.txt
                                  k8s_privileged_action.json   ← snapshot
  k8s_rbac.yaml                 → k8s_rbac_digest.txt
  k8s_crd.yaml                  → k8s_crd_digest.txt
  tf_create.json                → tf_create_digest.txt
  tf_destroy.json               → tf_destroy_digest.txt
  tf_mixed.json                 → tf_mixed_digest.txt
                                  tf_mixed_action.json         ← snapshot
  tf_module.json                → tf_module_digest.txt
                                  tf_module_action.json        ← snapshot
  helm_output.yaml              → helm_output_digest.txt
```

Snapshots exist for cases that exercise non-trivial logic:
- `k8s_multidoc` — resource_count from multi-doc, sorting
- `k8s_privileged` — risk_tags from catastrophic detector
- `tf_mixed` — multiple resource types, mixed actions, resource_count
- `tf_module` — module path normalization (type+name, not address)

Test logic:

```go
var update = flag.Bool("update", false, "update golden files (requires EVIDRA_UPDATE_GOLDEN=1)")

func TestGolden(t *testing.T) {
    if *update && os.Getenv("EVIDRA_UPDATE_GOLDEN") != "1" {
        t.Fatal("-update requires EVIDRA_UPDATE_GOLDEN=1 env var")
    }

    entries, _ := os.ReadDir("tests/golden")
    for _, e := range entries {
        ext := filepath.Ext(e.Name())
        if ext != ".yaml" && ext != ".json" {
            continue
        }
        t.Run(e.Name(), func(t *testing.T) {
            input, _ := os.ReadFile("tests/golden/" + e.Name())
            base := strings.TrimSuffix(e.Name(), ext)
            digestPath := "tests/golden/" + base + "_digest.txt"
            actionPath := "tests/golden/" + base + "_action.json"

            result, err := Canonicalize(input)
            require.NoError(t, err)

            if *update {
                os.WriteFile(digestPath,
                    []byte(result.IntentDigest+"\n"), 0644)
                actionJSON, _ := json.Marshal(result.CanonicalAction)
                os.WriteFile(actionPath, actionJSON, 0644)
                return
            }

            // Always check digest
            expected, _ := os.ReadFile(digestPath)
            assert.Equal(t,
                strings.TrimSpace(string(expected)),
                result.IntentDigest,
                "intent_digest mismatch")

            // Check action snapshot if it exists
            if expectedAction, err := os.ReadFile(actionPath); err == nil {
                actualAction, _ := json.Marshal(result.CanonicalAction)
                assert.JSONEq(t,
                    string(expectedAction),
                    string(actualAction),
                    "canonical_action mismatch — check resource_count, scope_class, operation_class, risk_tags")
            }
        })
    }
}
```

This catches: intent_digest changing (always), AND resource_count /
scope_class / risk_tags breaking silently (for key cases).

`-update` requires `EVIDRA_UPDATE_GOLDEN=1` env var. Prevents
accidental golden overwrites in CI. Version bump command becomes:

```
EVIDRA_UPDATE_GOLDEN=1 go test -run TestGolden -update
```

### Test B: Noise Immunity (5 mutators)

Take each golden input. Mutate it 5 ways. Assert same digest.

```go
func TestNoiseImmunity(t *testing.T) {
    entries := loadGoldenInputs()
    for _, e := range entries {
        input, _ := os.ReadFile(e.Path)
        baseline, _ := Canonicalize(input)

        t.Run(e.Name+"/reorder_fields", func(t *testing.T) {
            mutated := reorderYAMLFields(input)
            result, _ := Canonicalize(mutated)
            assert.Equal(t, baseline.IntentDigest, result.IntentDigest)
        })

        t.Run(e.Name+"/reorder_docs", func(t *testing.T) {
            mutated := reorderMultiDoc(input)
            result, _ := Canonicalize(mutated)
            assert.Equal(t, baseline.IntentDigest, result.IntentDigest)
        })

        t.Run(e.Name+"/add_whitespace", func(t *testing.T) {
            mutated := addRandomWhitespace(input)
            result, _ := Canonicalize(mutated)
            assert.Equal(t, baseline.IntentDigest, result.IntentDigest)
        })

        t.Run(e.Name+"/add_noise_annotations", func(t *testing.T) {
            mutated := addNoiseAnnotations(input)
            result, _ := Canonicalize(mutated)
            assert.Equal(t, baseline.IntentDigest, result.IntentDigest)
        })

        t.Run(e.Name+"/add_status", func(t *testing.T) {
            mutated := addStatusBlock(input)
            result, _ := Canonicalize(mutated)
            assert.Equal(t, baseline.IntentDigest, result.IntentDigest)
        })
    }
}
```

5 mutators. 50 subtests total (10 inputs × 5 mutations).

**Mutator safety rules:** Noise mutators must be minimal and dumb.
Text-level operations only (insert whitespace, swap YAML lines,
append annotation strings). Do NOT round-trip through yaml.v3
for noise mutations — the parser itself may alter types (`1` vs
`"1"`, YAML anchors). If a mutator introduces its own bugs, it
becomes a maintenance burden, not a safety net.

Terraform golden cases are JSON. Noise mutators that manipulate
YAML (reorder_fields, reorder_docs, add_status) skip JSON inputs
automatically.

### Test C: Shape Hash Sensitivity (1 test)

One test proving resource_shape_hash reacts to spec changes.

```go
func TestShapeHashSensitivity(t *testing.T) {
    input, _ := os.ReadFile("tests/golden/k8s_deployment.yaml")
    baseline, _ := Canonicalize(input)

    // Change image tag in the raw YAML
    mutated := bytes.Replace(input,
        []byte("api-server:v2.4.1"),
        []byte("api-server:v2.5.0"),
        1)
    result, err := Canonicalize(mutated)
    require.NoError(t, err)

    // Intent must NOT change (same resource identity)
    assert.Equal(t, baseline.IntentDigest, result.IntentDigest,
        "intent should be stable across image tag changes")

    // Shape MUST change (spec content differs)
    assert.NotEqual(t, baseline.ShapeHash, result.ShapeHash,
        "shape_hash must detect spec changes")
}
```

This single test validates the entire shape_hash contract: identity
is stable, content detection works. No test suite needed.

### Test D: Crash Safety Fuzz (P1, not P0)

Go native fuzz seeded from golden inputs. Goal: no panics, no
hangs, no OOM. Not correctness — just "don't crash on weird input."

```go
func FuzzCanonicalize(f *testing.F) {
    entries, _ := os.ReadDir("tests/golden")
    for _, e := range entries {
        ext := filepath.Ext(e.Name())
        if ext == ".yaml" || ext == ".json" {
            raw, _ := os.ReadFile("tests/golden/" + e.Name())
            f.Add(raw)
        }
    }

    f.Fuzz(func(t *testing.T, input []byte) {
        Canonicalize(input)  // must not panic
    })
}
```

Add this when the adapter is stable. Not for v0.3.0.
Run locally with `go test -fuzz=FuzzCanonicalize -fuzztime=30s`.

---

## 2. What We Dropped and Why

| Previous proposal | Status |
|-------------------|--------|
| 3000 noise mutations per adapter | Dropped. 5 mutators × 10 inputs = 50 tests. Same bugs. |
| Semantic mutation suite | Dropped. Replaced by 1 shape_hash sensitivity test. |
| Identity mutation suite | Dropped. Identity = 4 strings hashed. Trivially correct. |
| Cross-version boundary tests | Deferred. Add when v2 exists. |
| Structured fuzz generator | Deferred. Go native fuzz from golden seeds is enough. |
| Performance benchmarks | Deferred. Profile when slow. |
| metadata.json per corpus case | Dropped. Comment in code is enough. |
| Full action snapshot per case | **Kept for 3-5 key cases.** Catches resource_count, scope_class, risk_tags regressions. |
| Crash safety fuzz | **Kept as P1.** Cheap with Go native fuzz. Not for v0.3.0. |

---

## 3. When to Add More Tests

Add a golden case when:
- New resource type with unusual structure
- Bug fix where digest was wrong

Add a noise mutator when:
- New noise field discovered in production

Add fuzz testing when:
- Crash found in production

Do not add tests preemptively.

---

## 4. Version Bump Process

```
1. Change the adapter code
2. go test → golden tests fail (digest and/or action snapshot)
3. EVIDRA_UPDATE_GOLDEN=1 go test -run TestGolden -update
4. git diff tests/golden/ → review: are changes expected?
   - Check digest changes (intent identity shifted?)
   - Check action snapshot changes (resource_count? scope_class? risk_tags?)
5. Bump canonicalization_version in adapter
6. git commit -m "canon: bump k8s/v2, reason: ..."
```

The `EVIDRA_UPDATE_GOLDEN=1` env var prevents accidental overwrites.
CI never sets this variable. A developer who runs `-update` without
the env var gets a clear error.

---

## 5. CI

```yaml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test ./... -v -count=1
```

No special CI jobs. No nightly fuzz. No weekly benchmarks.
Add those when needed.

---

## 6. Total

| Test | Cases | Lines of code |
|------|-------|---------------|
| Golden corpus (digest) | 10 | 40 |
| Golden corpus (action snapshot) | 3-5 | (same test, +JSONEq assert) |
| Noise immunity | 50 | 40 |
| Shape hash sensitivity | 1 | 15 |
| Crash fuzz (P1) | 1 | 10 |
| **Total** | **~65** | **~105** |

~105 lines of test code. 10 input files + 10 digests + 4 action
snapshots. Catches digest regressions, canonical_action field
regressions, noise immunity failures, shape_hash sensitivity,
and crash bugs. Maintainable by one person.
