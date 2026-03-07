# Evidra Experiment — Workstation Execution Guide (macOS)

Dedicated workstation for signal validation, agent experiments, and dataset collection.
Colima + k3d for minimal resource usage. Three phases, one machine.

---

## 1. Hardware Requirements

Any macOS workstation from ~2018+. The experiment is IO-bound (API calls + kubectl), not compute-heavy.

| Spec | Minimum | Recommended |
|------|---------|-------------|
| RAM | 8 GB | 16 GB |
| Disk | 10 GB free | 20 GB free |
| CPU | Any | Apple Silicon (M1+) |
| macOS | Monterey 12+ | Sonoma 14+ |
| Network | Stable WiFi | Wired preferred |

---

## 2. Provision Environment

Open Terminal. Copy-paste blocks one at a time.

### Step 1: Homebrew

```bash
command -v brew >/dev/null || /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

Apple Silicon Macs — if brew just installed, run the line it tells you:
```bash
echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zshrc
eval "$(/opt/homebrew/bin/brew shellenv)"
```

### Step 2: Core Tools

```bash
brew install colima k3d kubectl helm jq go python3
```

| Tool | Purpose |
|------|---------|
| colima | Lightweight Docker runtime (replaces Docker Desktop) |
| k3d | k3s-in-Docker cluster manager |
| kubectl | Kubernetes CLI |
| helm | Helm scenarios + chart rendering for dataset |
| jq | JSON parsing in all scripts |
| go | Build Evidra from source |
| python3 | Agent experiment runner |

### Step 3: Start Colima

```bash
colima start --memory 4 --cpu 2 --disk 20
docker ps
```

If `Cannot connect to the Docker daemon`:
```bash
colima status
docker context use colima
docker ps
```

Optional auto-start: `brew services start colima`

### Step 4: Create k3d Cluster

```bash
k3d cluster create evidra-bench --agents 0
kubectl get nodes
```

Expected:
```
NAME                        STATUS   ROLES                  AGE   VERSION
k3d-evidra-bench-server-0   Ready    control-plane,master   15s   v1.31.x+k3s1
```

### Step 5: Build Evidra

```bash
cd ~
git clone https://github.com/vitas/evidra-benchmark.git
cd evidra-benchmark
make build

echo 'export PATH="$HOME/evidra-benchmark/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc

evidra version
```

### Step 6: Verify Everything

```bash
echo "--- Checking all dependencies ---"
colima status        && echo "✓ colima" || echo "✗ colima"
docker ps >/dev/null && echo "✓ docker" || echo "✗ docker"
k3d version          && echo "✓ k3d"    || echo "✗ k3d"
kubectl get nodes >/dev/null && echo "✓ kubectl" || echo "✗ kubectl"
helm version         && echo "✓ helm"   || echo "✗ helm"
evidra version       && echo "✓ evidra" || echo "✗ evidra"
command -v jq >/dev/null && echo "✓ jq" || echo "✗ jq"
python3 -c "import json, urllib.request; print('✓ python stdlib http client')"
echo "--- Done ---"
```

All lines must show ✓.

---

| Provider (--provider) | --model-id example | API key env var |
|---|---|---|
| claude (headless CLI) | claude/haiku | not required (CLI login required) |
| anthropic | anthropic/claude-3-5-haiku | ANTHROPIC_API_KEY |
| openai | openai/gpt-4o-mini | OPENAI_API_KEY |
| deepseek | deepseek/deepseek-chat | DEEPSEEK_API_KEY |
| gemini | gemini/gemini-2.5-flash-lite | GEMINI_API_KEY |
| openrouter | openrouter/<model> | OPENROUTER_API_KEY |
| ollama (local) | ollama/llama3.2 | not required |

Sources:

- Gemini pricing/billing: https://ai.google.dev/pricing , https://ai.google.dev/gemini-api/docs/billing/
- OpenRouter free router: https://openrouter.ai/docs/guides/routing/routers/free-models-router
- Bifrost OpenAI SDK integration: https://docs.getbifrost.ai/integrations/openai-sdk/overview
- Ollama local auth-free API: https://docs.ollama.com/api/authentication
- Anthropic prepaid credits: https://support.anthropic.com/en/articles/8977456-how-do-i-pay-for-my-claude-api-usage
- OpenAI ChatGPT vs API billing: https://help.openai.com/en/articles/9039756-billing-settings-in-chatgpt-vs-platform

## 3. API Keys

```bash
mkdir -p ~/.evidra-experiment

cat > ~/.evidra-experiment/.env << 'ENVEOF'
# DeepSeek — $0.27/1M input — primary target (known loop failures from SWE-EVO)
export DEEPSEEK_API_KEY=""

# OpenAI — $0.15/1M input (gpt-4o-mini)
export OPENAI_API_KEY=""

# Anthropic — $0.80/1M input (claude-haiku-4-5)
export ANTHROPIC_API_KEY=""

# Google — $0.10/1M input (gemini-2.0-flash) — free tier works
export GEMINI_API_KEY=""

# OpenRouter (optional) — Qwen, Mistral, others
export OPENROUTER_API_KEY=""
ENVEOF

chmod 600 ~/.evidra-experiment/.env
nano ~/.evidra-experiment/.env   # paste your keys
```

Minimum 2 keys needed. DeepSeek + one other.

Test:
```bash
source ~/.evidra-experiment/.env
echo "keys loaded"
```

### Optional: Bifrost Gateway Runner

If you run a local Bifrost gateway, use the `evidra-exp` Bifrost adapter:

```bash
export EVIDRA_BIFROST_BASE_URL="http://localhost:8080/openai"
# optional Bifrost headers
# export EVIDRA_BIFROST_VK="vk_..."
# export EVIDRA_BIFROST_AUTH_BEARER="..."

go run ./cmd/evidra-exp artifact run \
  --model-id anthropic/claude-3-5-haiku \
  --provider bifrost \
  --agent bifrost \
  --mode local-mcp \
  --prompt-file prompts/experiments/runtime/system_instructions.txt \
  --repeats 1 \
  --max-cases 1 \
  --timeout-seconds 300
```

In this mode credentials can be handled by Bifrost configuration or by Bifrost-specific headers above.

Execution-mode smoke (MCP + real kubectl against current kube context):

```bash
go run ./cmd/evidra-exp execution run \
  --model-id execution/mcp-kubectl \
  --provider local \
  --agent mcp-kubectl \
  --mode local-mcp \
  --repeats 1 \
  --max-scenarios 1 \
  --timeout-seconds 600
```

---

## 4. Project Structure

```bash
mkdir -p ~/evidra-experiment
cd ~/evidra-experiment
```

Full layout after all three phases:

```
~/evidra-experiment/
├── .env -> ~/.evidra-experiment/.env
│
├── # ── Phase 1: Fault Injection (FAULT_INJECTION_RUNBOOK.md) ──
├── helpers.sh                        # Shared functions: prescribe, report, check_signal
├── scenarios/
│   ├── f01-retry-loop.sh             # 13 fault injection scenarios
│   ├── f02-variant-retry.sh
│   ├── f03-stalled-operation.sh
│   ├── f04-unprescribed-action.sh
│   ├── f05-artifact-drift.sh
│   ├── f06-blast-radius.sh
│   ├── f07-new-scope-tool-switch.sh
│   ├── f08-duplicate-report.sh
│   ├── f09-cross-actor.sh
│   ├── f10-risk-privileged.sh
│   ├── f11-risk-hostpath.sh
│   ├── f12-risk-host-namespace.sh
│   └── f13-clean-session.sh
├── run_all_faults.sh                 # Master fault injection runner
│
├── # ── Phase 2: Agent Experiment (EVIDRA_EXPERIMENT_DESIGN_V1.md) ──
├── evidra-exp                        # Go CLI binary (artifact/execution runners)
├── experiment-scenarios/
│   ├── scenario-01.yaml              # 10 adversarial agent scenarios
│   ├── scenario-02.yaml
│   └── ...
├── fixtures/
│   ├── base/
│   │   ├── working-nginx.yaml
│   │   └── agent-rbac.yaml
│   ├── scenario-01/
│   │   └── crash-app-deployment.yaml
│   └── ...
├── scripts/
│   ├── setup_scenario.sh             # Per-scenario cluster reset
│   └── run_all_agents.sh             # Master agent experiment runner
│
├── # ── Phase 3: Dataset Collection (EVIDRA_DATASET_ARCHITECTURE.md) ──
├── corpus/                           # Layer A: raw artifacts (greedy, immutable)
│   ├── k8s/
│   │   ├── privileged_container/
│   │   ├── hostpath_mount/
│   │   └── host_namespace_escape/
│   ├── tf/
│   │   ├── iam_wildcard/
│   │   └── s3_public_access/
│   └── sarif/
├── cases/                            # Layer B: curated benchmark cases
│   ├── k8s-privileged-container-fail/
│   │   ├── README.md
│   │   ├── expected.json
│   │   └── golden/
│   │       └── contract.json
│   └── ...
│
├── # ── Results ──
└── results/
    ├── fault-injection/              # Phase 1 output
    │   ├── evidence-*/
    │   └── summary.txt
    ├── agents/                       # Phase 2 output
    │   ├── scenario-01/
    │   │   ├── deepseek-chat/
    │   │   │   └── run-20260310/
    │   │   │       ├── evidence/     # Evidra evidence chain
    │   │   │       ├── transcript.jsonl
    │   │   │       ├── scorecard.json
    │   │   │       ├── signals.json
    │   │   │       └── metadata.json
    │   │   ├── gpt-4o-mini/
    │   │   └── ...
    │   └── ...
    └── dataset-validation/           # Phase 3 output
```

Initialize:
```bash
cd ~/evidra-experiment
ln -sf ~/.evidra-experiment/.env .env
mkdir -p scenarios fixtures/base scripts results/{fault-injection,agents,dataset-validation}
mkdir -p corpus/{k8s/{privileged_container,hostpath_mount,host_namespace_escape},tf/{iam_wildcard,s3_public_access},sarif}
mkdir -p cases
```

---

## 5. Three Execution Phases

### Phase 1: Fault Injection (no LLM, no cluster needed for most)

Validates that Evidra detectors fire correctly on known faults. Scripted sequences, no AI, deterministic. Run first.

**Source:** `FAULT_INJECTION_RUNBOOK.md`

```bash
cd ~/evidra-experiment
export EVIDRA_SIGNING_MODE=optional
export WORKSPACE="$HOME/evidra-experiment"

# Run all 13 fault injection scenarios
bash run_all_faults.sh
```

Duration: ~2 minutes.
Cost: $0 (no API calls).
Output: `results/fault-injection/` with evidence chains per scenario.

**Success criteria:** 12 of 13 scenarios PASS (F02 is a known gap).

### Phase 2: Agent Experiment (needs LLM keys + cluster)

Real AI agents through adversarial infrastructure scenarios. Records evidence via evidra prescribe/report.

**Source:** `EVIDRA_EXPERIMENT_DESIGN_V1.md`

```bash
cd ~/evidra-experiment
source .env

# Ensure cluster is running
kubectl get nodes

# Run all models × all scenarios
caffeinate -dims bash scripts/run_all_agents.sh 2>&1 | tee results/agents/experiment.log
```

Duration: 3-4 hours (leave overnight).
Cost: ~$1-5 total across all models.
Output: `results/agents/` with evidence chains, transcripts, scorecards per model × scenario.

**Result data per run:**

```
results/agents/scenario-01/deepseek-chat/run-20260310/
  evidence/                    # Evidra JSONL evidence chain
  transcript.jsonl             # Full LLM conversation log
  scorecard.json               # evidra scorecard output
  signals.json                 # evidra explain output
  metadata.json                # model, tokens, cost, duration
```

**metadata.json format:**

```json
{
  "model": "deepseek/deepseek-chat",
  "scenario": "scenario-01",
  "total_tokens_in": 15240,
  "total_tokens_out": 4820,
  "duration_seconds": 187.3,
  "turns": 24,
  "timestamp": "2026-03-10T22:14:00Z",
  "context": {
    "session_kind": "experiment",
    "origin": "cli"
  }
}
```

**transcript.jsonl format (one JSON per line):**

```json
{"turn": 3, "timestamp": "2026-03-10T22:15:01Z", "role": "assistant", "content": "I'll apply the deployment...", "tool_calls": [{"name": "evidra_prescribe", "args": {"tool": "kubectl", "operation": "apply"}}, {"name": "shell_exec", "args": {"command": "kubectl apply -f /tmp/deployment.yaml"}}], "model": "deepseek/deepseek-chat", "tokens_in": 1240, "tokens_out": 180}
```

### Phase 3: Dataset Collection (needs Evidra + corpus sources)

Collect artifacts from OSS repos, create benchmark cases with ground truth.

**Source:** `EVIDRA_DATASET_ARCHITECTURE.md`, `DATASET_COLLECTOR_SKILL.md`

```bash
cd ~/evidra-experiment

# Option A: Use AI agent with collector skill to gather artifacts
# (give DATASET_COLLECTOR_SKILL.md as context to Claude Code)

# Option B: Manual collection
# 1. Download artifacts into corpus/
curl -sL "https://raw.githubusercontent.com/kubescape/regolibrary/main/.../test.yaml" \
  > corpus/k8s/privileged_container/kubescape-C0057.yaml

# 2. Create case
bash scripts/bench-add.sh k8s-privileged-container-fail \
  --artifact corpus/k8s/privileged_container/kubescape-C0057.yaml \
  --source kubescape-regolibrary

# 3. Run detector, create golden
evidra prescribe --tool kubectl --operation apply \
  --artifact corpus/k8s/privileged_container/kubescape-C0057.yaml \
  --signing-mode optional | jq '{
    ground_truth_pattern: "k8s.privileged_container",
    risk_level: .risk_level,
    risk_tags: (.risk_tags // []),
    operation_class: .operation_class,
    scope_class: .scope_class,
    resource_count: .resource_count,
    canon_version: .canon_version,
    evidra_version: "0.5.0"
  }' > cases/k8s-privileged-container-fail/golden/contract.json

# 4. Validate
bash tests/benchmark/scripts/validate-dataset.sh
```

**expected.json format:**

```json
{
  "case_id": "k8s-privileged-container-fail",
  "case_kind": "artifact",
  "category": "kubernetes",
  "difficulty": "medium",
  "ground_truth_pattern": "k8s.privileged_container",
  "ground_truth": {
    "infrastructure_risk": "critical",
    "blast_radius_resources": 1,
    "security_impact": "high",
    "attack_surface": "container-escape"
  },
  "artifact_ref": "corpus/k8s/privileged_container/kubescape-C0057.yaml",
  "artifact_digest": "sha256:a1b2c3...",
  "risk_details_expected": ["k8s.privileged_container"],
  "risk_level": "critical",
  "signals_expected": {},
  "tags": ["kubernetes", "security", "privileged-container"],
  "processing": {
    "evidra_version": "0.5.0",
    "processed_at": "2026-03-20T14:00:00Z"
  },
  "source_refs": [
    { "source_id": "kubescape-regolibrary", "composition": "real-derived" }
  ]
}
```

**golden/contract.json format:**

```json
{
  "ground_truth_pattern": "k8s.privileged_container",
  "risk_level": "critical",
  "risk_tags": ["k8s.privileged_container"],
  "operation_class": "mutate",
  "scope_class": "unknown",
  "resource_count": 1,
  "canon_version": "k8s/v1",
  "evidra_version": "0.5.0"
}
```

---

## 6. Daily Operations

### Start of Day

```bash
colima start
kubectl get nodes
```

If cluster gone: `k3d cluster create evidra-bench --agents 0`

### Run Experiments

```bash
cd ~/evidra-experiment

# Phase 1 (fast, do first)
bash run_all_faults.sh

# Phase 2 (slow, leave overnight)
source .env
caffeinate -dims bash scripts/run_all_agents.sh 2>&1 | tee results/agents/experiment.log
```

### Monitor (second terminal)

```bash
tail -f ~/evidra-experiment/results/agents/experiment.log
```

### Collect Results

```bash
cd ~/evidra-experiment
tar -czf "results-$(date +%Y%m%d).tar.gz" results/
```

Transfer: AirDrop / `scp` / USB / `git push` to private repo.

### Quick Analysis

```bash
# Signal heatmap after agent experiment
echo "scenario | model | signals"
for f in results/agents/*/*/*/signals.json; do
  scenario=$(echo "$f" | cut -d/ -f3)
  model=$(echo "$f" | cut -d/ -f4)
  signals=$(jq -r '[.signals[] | select(.count > 0) | "\(.signal)(\(.count))"] | join(", ")' "$f" 2>/dev/null)
  echo "$scenario | $model | ${signals:-none}"
done

# Total cost
find results/agents -name metadata.json -exec jq '.total_tokens_in + .total_tokens_out' {} \; \
  | awk '{sum+=$1} END {printf "Total tokens: %d (~$%.2f at $0.50/1M)\n", sum, sum/1000000*0.5}'
```

---

## 7. Cluster Management

```bash
# Namespace reset between scenarios (2-3 seconds)
kubectl delete ns bench-app bench-cleanup bench-staging bench-monitoring --ignore-not-found --wait=false
kubectl create ns bench-app

# Nuclear reset (15 seconds)
k3d cluster delete evidra-bench && k3d cluster create evidra-bench --agents 0

# Resource usage
docker stats --no-stream
colima status
```

---

## 8. Troubleshooting

| Problem | Fix |
|---------|-----|
| colima not running | `colima start` |
| docker connection refused | `docker context use colima` |
| kubectl connection refused | `k3d cluster start evidra-bench` |
| nodes NotReady | Wait 15s |
| Bifrost timeout | Check gateway/provider status; skip model |
| Bifrost 401/403 | Re-check Bifrost auth headers or gateway auth config |
| Bifrost rate limit | Add `sleep 5` between runs |
| Mac sleeping | Always use `caffeinate -dims` |
| Disk full | `du -sh results/*` then remove old runs |
| `evidra: not found` | `source ~/.zshrc` |
| Fault injection F03 fails | TTL issue — ensure `FAULT_TTL=1s` in helpers.sh |
| Agent runner hangs | Set `--timeout-seconds 300` in `evidra-exp ... run` |

### Full Reset

```bash
k3d cluster delete evidra-bench
colima stop && colima delete
colima start --memory 4 --cpu 2 --disk 20
k3d cluster create evidra-bench --agents 0
kubectl get nodes
```

---

## 9. Resource Footprint

| Component | RAM | CPU (idle) | Disk |
|-----------|-----|------------|------|
| Colima VM | ~1.0 GB | ~1% | ~2 GB |
| k3s server | ~0.4 GB | ~2% | ~0.5 GB |
| Python runner | ~0.1 GB | ~1% | negligible |
| **Total** | **~1.5 GB** | **~4%** | **~2.5 GB** |

| Setup | RAM | Startup |
|-------|-----|---------|
| Docker Desktop + Kind (3 nodes) | ~5 GB | 45-60s |
| Docker Desktop + Kind (1 node) | ~3.5 GB | 30-45s |
| Minikube | ~2.5 GB | 30-40s |
| **Colima + k3d (1 node)** | **~1.5 GB** | **10-15s** |

---

## 10. Document Map

This machine runs everything described in these documents:

| Document | What It Does | Phase |
|----------|-------------|-------|
| `docs/research/FAULT_INJECTION_RUNBOOK.md` | 13 scripted detector tests | Phase 1 |
| `docs/research/EVIDRA_EXPERIMENT_DESIGN_V1.md` | 10 scenarios × 4-5 models | Phase 2 |
| `docs/research/DATASET_COLLECTOR_SKILL.md` | Agent skill for corpus collection | Phase 3 |
| `docs/system-design/EVIDRA_DATASET_ARCHITECTURE.md` | Case structure, golden files, versioning | Phase 3 |
| `docs/research/AI_AGENT_FAILURE_PATTERNS.md` | Signal theory — reference only | — |
| `docs/research/EVIDRA_AGENT_SKILL.md` | Agent protocol skill — used by experiment agents | Phase 2 |
| `docs/research/COMMUNITY_BENCHMARK_DESIGN.md` | Future contribution system — not yet | — |

**Execution order: Phase 1 → Phase 2 → Phase 3.** Each phase validates assumptions needed by the next.
