# v0.5.0 — Platform Layer (API + Auth + DB + Receipts)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate REST API, auth middleware, and database layer from `evidra-mcp` to create `evidra-api` — a hosted evidence collection service with forward integrity (receipts), OIDC actor verification, multi-tenancy, and agent version comparison.

**Architecture:** Reuse infrastructure primitives from `../evidra-mcp` (API scaffolding, auth middleware, DB migrations, key store), but rewrite all handlers to use benchmark semantics (prescribe/report, not validate). The API server receives evidence entries via HTTP, stores in Postgres, returns signed receipts, and serves scorecards.

**Tech Stack:** Go 1.24, `github.com/jackc/pgx/v5` (Postgres), stdlib `net/http`, existing evidence/signal/score packages.

**Prerequisites:** v0.4.0 signing integration must be complete (entries are signed before forwarding).

---

## Phase 1: API Server Scaffolding

### Task 1: Bootstrap cmd/evidra-api

**Files:**
- Create: `cmd/evidra-api/main.go`
- Reuse from: `../evidra-mcp/cmd/evidra-api/main.go` (structure only)

**What to copy and adapt:**
- Configuration from environment: `DATABASE_URL`, `EVIDRA_API_KEY`, `EVIDRA_SIGNING_KEY`, `EVIDRA_SIGNING_KEY_PATH`, `EVIDRA_ENVIRONMENT`, `PORT`
- Signer initialization pattern
- Graceful shutdown with `signal.NotifyContext`
- Do NOT copy OPA engine or policy references

**Server config struct:**

```go
type Config struct {
    Port            string
    DatabaseURL     string
    APIKey          string
    SigningKey       string
    SigningKeyPath   string
    Environment     string
    TenantMode      bool   // require tenant_id on all requests
}
```

**Endpoints to register:**

```
GET  /healthz                    — health check
GET  /v1/evidence/pubkey         — signing public key (PEM)
POST /v1/evidence/forward        — receive and store evidence entry, return receipt
GET  /v1/evidence/entries        — list evidence entries (paginated, filtered)
GET  /v1/evidence/entries/:id    — get single entry by entry_id
GET  /v1/evidence/scorecard      — compute scorecard from stored evidence
POST /auth/check                 — verify auth token
```

**Step: Build skeleton, commit**

```bash
go build ./cmd/evidra-api/
git add cmd/evidra-api/
git commit -m "feat: bootstrap evidra-api server with config and routing"
```

---

### Task 2: Database layer

**Files:**
- Create: `internal/db/db.go` — connection pool + migrations
- Create: `internal/db/migrations/001_evidence_entries.sql`
- Reuse from: `../evidra-mcp/internal/db/db.go` (Connect + runMigrations pattern)

**Schema:**

```sql
CREATE TABLE IF NOT EXISTS evidence_entries (
    id            BIGSERIAL PRIMARY KEY,
    entry_id      TEXT NOT NULL UNIQUE,
    tenant_id     TEXT NOT NULL DEFAULT '',
    type          TEXT NOT NULL,
    trace_id      TEXT NOT NULL,
    actor_type    TEXT NOT NULL DEFAULT '',
    actor_id      TEXT NOT NULL DEFAULT '',
    timestamp     TIMESTAMPTZ NOT NULL,
    intent_digest   TEXT,
    artifact_digest TEXT,
    payload       JSONB NOT NULL,
    previous_hash TEXT NOT NULL DEFAULT '',
    hash          TEXT NOT NULL,
    signature     TEXT NOT NULL DEFAULT '',
    spec_version  TEXT NOT NULL DEFAULT '',
    canon_version TEXT NOT NULL DEFAULT '',
    adapter_version TEXT NOT NULL DEFAULT '',
    scoring_version TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_entries_tenant_id ON evidence_entries(tenant_id);
CREATE INDEX idx_entries_actor_id ON evidence_entries(actor_id);
CREATE INDEX idx_entries_type ON evidence_entries(type);
CREATE INDEX idx_entries_trace_id ON evidence_entries(trace_id);
CREATE INDEX idx_entries_timestamp ON evidence_entries(timestamp);
```

**Step: Create migration, wire Connect, commit**

```bash
go build ./cmd/evidra-api/
git add internal/db/
git commit -m "feat: add database layer with evidence_entries schema"
```

---

### Task 3: Auth middleware

**Files:**
- Create: `internal/auth/middleware.go`
- Create: `internal/auth/context.go`
- Reuse from: `../evidra-mcp/internal/auth/middleware.go`

**What to copy:**
- `StaticKeyMiddleware(apiKey string)` — Phase 0 static API key
- `KeyStoreMiddleware(store KeyLookup)` — Phase 1 DB-backed keys
- `WithTenantID()` / `TenantID()` context functions
- `extractBearerToken()`, `authFail()`, `jitterSleep()`

**What to add for v0.5.0:**
- `OIDCMiddleware(issuerURL string)` — validates JWT tokens, extracts actor identity
- Sets `AuthContext` on Actor from verified token claims

```go
type AuthContext struct {
    Verified bool   `json:"verified"`
    Issuer   string `json:"issuer,omitempty"`
    Subject  string `json:"subject,omitempty"`
    Method   string `json:"method"` // "api_key", "oidc", "none"
}
```

**Step: Copy middleware, add OIDC stub, commit**

```bash
git add internal/auth/
git commit -m "feat: add auth middleware with static key and OIDC support"
```

---

## Phase 2: Forward Integrity + Receipts

### Task 4: Evidence forward handler

**Files:**
- Create: `internal/api/forward_handler.go`

**Endpoint:** `POST /v1/evidence/forward`

**Request:** JSON body = `EvidenceEntry` (as produced by MCP server or CLI)

**Handler logic:**
1. Parse request body as `EvidenceEntry`
2. Validate entry: type is valid, hash recomputes correctly, signature verifies (if present)
3. Verify `previous_hash` matches last entry in DB (or is empty for first entry per tenant)
4. Insert into `evidence_entries` table
5. Build receipt entry:
   - Type: `EntryTypeReceipt`
   - Payload: `{"received_entry_id": "<forwarded entry_id>", "server_timestamp": "...", "sequence": N}`
   - Sign receipt with server's signing key
6. Insert receipt entry into DB
7. Return receipt entry as response

**Receipt links back to forwarded entry:**

```go
type ReceiptPayload struct {
    ReceivedEntryID string `json:"received_entry_id"`
    ServerTimestamp  string `json:"server_timestamp"`
    Sequence        int64  `json:"sequence"`
}
```

**Step: Implement handler, test, commit**

```bash
go test ./internal/api/ -v -count=1
git add internal/api/
git commit -m "feat: evidence forward handler with signed receipts"
```

---

### Task 5: Wire forward path in MCP server

**Files:**
- Modify: `pkg/mcpserver/server.go` — add HTTP forwarder
- Modify: `cmd/evidra-mcp/main.go` — add `EVIDRA_API_URL` env var

**What to add:**
- `ForwardURL` field in `Options`
- After writing entry to local evidence store, POST to `ForwardURL + "/v1/evidence/forward"`
- Store returned receipt as local evidence entry (type: receipt)
- Forward is **best-effort** — local write always succeeds, forward failure is logged

```go
func (s *BenchmarkService) forwardEntry(entry evidencePkg.EvidenceEntry) {
    if s.forwardURL == "" {
        return
    }
    body, _ := json.Marshal(entry)
    resp, err := http.Post(s.forwardURL+"/v1/evidence/forward", "application/json", bytes.NewReader(body))
    if err != nil {
        log.Printf("forward error: %v", err)
        return
    }
    defer resp.Body.Close()
    if resp.StatusCode == 200 {
        var receipt evidencePkg.EvidenceEntry
        if json.NewDecoder(resp.Body).Decode(&receipt) == nil {
            evidencePkg.AppendEntryAtPath(s.evidencePath, receipt)
        }
    }
}
```

**Step: Wire forwarder, test, commit**

```bash
go test ./pkg/mcpserver/ -v -count=1
git add pkg/mcpserver/ cmd/evidra-mcp/
git commit -m "feat: wire evidence forwarding to remote API with receipt storage"
```

---

### Task 6: Wire forward path in CLI

**Files:**
- Modify: `cmd/evidra/main.go` — add `--api-url` flag to prescribe and report

**Same pattern as MCP server:** After local write, POST to API, store receipt locally.

**Step: Wire, test, commit**

```bash
go build ./cmd/evidra/
git add cmd/evidra/main.go
git commit -m "feat: add --api-url flag for evidence forwarding in CLI"
```

---

## Phase 3: Actor Identity + Multi-Tenancy

### Task 7: Add AuthContext to Actor

**Files:**
- Modify: `pkg/evidence/entry.go` — add AuthContext to Actor struct

```go
type Actor struct {
    Type        string       `json:"type"`
    ID          string       `json:"id"`
    Provenance  string       `json:"provenance"`
    AuthContext *AuthContext  `json:"auth_context,omitempty"`
}

type AuthContext struct {
    Verified bool   `json:"verified"`
    Issuer   string `json:"issuer,omitempty"`
    Subject  string `json:"subject,omitempty"`
    Method   string `json:"method"`
}
```

**Step: Update struct, ensure tests pass, commit**

```bash
go test ./... -count=1
git add pkg/evidence/
git commit -m "feat: add AuthContext to Actor for verified identity"
```

---

### Task 8: Confidence model considers actor verification

**Files:**
- Modify: `internal/score/score.go` — update `ComputeConfidence` to factor in auth

```go
func ComputeConfidence(externalPct, violationRate float64, actorVerified bool) Confidence {
    if violationRate > 0.10 {
        return Confidence{Level: "low", ScoreCeiling: 85}
    }
    if externalPct > 0.50 {
        return Confidence{Level: "medium", ScoreCeiling: 95}
    }
    if !actorVerified {
        return Confidence{Level: "medium", ScoreCeiling: 95}
    }
    return Confidence{Level: "high", ScoreCeiling: 100}
}
```

**Step: Update, test, commit**

```bash
go test ./internal/score/ -v -count=1
git add internal/score/
git commit -m "feat: confidence model considers actor verification level"
```

---

### Task 9: Multi-tenancy enforcement in API

**Files:**
- Modify: `internal/api/forward_handler.go` — require tenant_id in tenant mode
- Create: `internal/api/middleware.go` — tenant isolation middleware

**Logic:**
- When `TenantMode=true`, all requests must include `X-Tenant-ID` header (or extract from auth token)
- Forward handler rejects entries with mismatched tenant_id
- Query endpoints filter by tenant_id from auth context
- Cross-tenant access returns 403

**Step: Implement, test, commit**

```bash
go test ./internal/api/ -v -count=1
git add internal/api/
git commit -m "feat: multi-tenancy enforcement in API endpoints"
```

---

## Phase 4: Agent Version Comparison + Remaining Features

### Task 10: Add actor_meta to prescribe input

**Files:**
- Modify: `pkg/mcpserver/server.go` — add `ActorMeta` to `PrescribeInput`
- Modify: `pkg/evidence/payloads.go` — add `ActorMeta` to `PrescriptionPayload`

```go
type ActorMeta struct {
    AgentVersion string `json:"agent_version,omitempty"`
    ModelID      string `json:"model_id,omitempty"`
    PromptID     string `json:"prompt_id,omitempty"`
}
```

Store in prescription payload. Used by compare --versions.

**Step: Add field, test, commit**

```bash
go test ./pkg/mcpserver/ -v -count=1
git add pkg/mcpserver/ pkg/evidence/
git commit -m "feat: add actor_meta (agent_version, model_id) to prescribe input"
```

---

### Task 11: Compare --versions

**Files:**
- Modify: `cmd/evidra/main.go` — add `--versions` flag to cmdCompare
- Modify: `internal/pipeline/bridge.go` — extract actor_meta from evidence

**Logic:**
- `evidra compare --actor claude-code --versions v1.2,v1.3`
- Filter evidence entries by actor_id AND agent_version
- Compute per-version scorecards
- Always valid comparison (same actor, same workload, different versions)

**Step: Implement, test, commit**

```bash
go build ./cmd/evidra/
git add cmd/evidra/main.go internal/pipeline/
git commit -m "feat: add --versions flag for agent version comparison"
```

---

### Task 12: Compare --force and overlap warning

**Files:**
- Modify: `cmd/evidra/main.go` — add `--force` flag, print warning to stderr

**Logic:**
- Compute workload overlap as already done
- If overlap < 0.3 and `--force` not set, print warning to stderr:
  ```
  WARNING: Low workload overlap (12%).
    actor-a: kubectl (staging, production)
    actor-b: terraform (production)
  Use --force to compare anyway.
  ```
- If `--force` set or overlap >= 0.3, proceed normally

**Step: Implement, commit**

```bash
git add cmd/evidra/main.go
git commit -m "feat: add --force flag and overlap warning to compare"
```

---

### Task 13: Server-side scorecard endpoint

**Files:**
- Create: `internal/api/scorecard_handler.go`

**Endpoint:** `GET /v1/evidence/scorecard?actor=X&period=30d&tool=Y&scope=Z`

**Logic:**
- Query evidence_entries from DB with filters
- Convert to signal entries via pipeline bridge
- Compute grouped scorecard
- Return JSON

**Step: Implement, test, commit**

```bash
go test ./internal/api/ -v -count=1
git add internal/api/
git commit -m "feat: server-side scorecard endpoint"
```

---

## Task 14: Final verification

**Step 1: Build all binaries**

```bash
go build ./cmd/evidra/ ./cmd/evidra-mcp/ ./cmd/evidra-api/
```

**Step 2: All tests pass**

```bash
go test ./... -v -count=1
```

**Step 3: Race detector**

```bash
go test -race ./...
```

**Step 4: Integration test with real Postgres**

```bash
go test -tags integration ./internal/db/ ./internal/api/ -v
```

Target: 250+ tests passing, 3 binaries building.

---

## Reuse Map (from evidra-mcp)

| Source | Target | Adaptation |
|--------|--------|------------|
| `cmd/evidra-api/main.go` | `cmd/evidra-api/main.go` | Remove OPA engine, add benchmark service |
| `internal/auth/middleware.go` | `internal/auth/middleware.go` | Add OIDC validation |
| `internal/auth/context.go` | `internal/auth/context.go` | Copy as-is |
| `internal/db/db.go` | `internal/db/db.go` | Copy Connect + migrations pattern |
| `internal/api/router.go` | `internal/api/router.go` | New routes for benchmark endpoints |
| `internal/api/pubkey_handler.go` | `internal/api/pubkey_handler.go` | Copy as-is |
| `internal/api/validate_handler.go` | — | Do NOT copy (policy-era, replaced by forward_handler) |

## Do NOT reuse

| Module | Reason |
|--------|--------|
| `pkg/policy/*` | Policy-era, not benchmark semantics |
| `pkg/runtime/*` | OPA runtime, not needed |
| `pkg/validate/*` | Validate-first, not observe-first |
| `pkg/bundlesource/*` | OPA bundle loading |
| `internal/engine/*` | OPA evaluation engine |
