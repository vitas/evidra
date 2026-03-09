import { useState } from "react";
import { CodeBlock } from "../components/CodeBlock";
import { MermaidDiagram } from "../components/MermaidDiagram";

const PIPELINE_CHART = `flowchart LR
  A["Raw Artifact"] --> B{"Adapter<br/>Selection"}
  B -->|K8s| C1["K8s Adapter"]
  B -->|Terraform| C2["TF Adapter"]
  B -->|Docker| C3["Docker Adapter"]
  B -->|Other| C4["Generic Adapter"]
  C1 & C2 & C3 & C4 --> D["CanonicalAction"]
  D --> E["Risk Matrix"]
  E --> F["Prescription"]
  F --> G[("Evidence<br/>Chain")]
  H["Exit Code"] --> I["Report"]
  I --> G
  G --> J["Signal Detectors<br/>8 signals"]
  J --> K["Scoring Engine"]
  K --> L["Scorecard<br/>+ Band"]`;

const SYSTEM_CHART = `flowchart TB
  subgraph Clients ["Client Layer"]
    CLI["evidra CLI<br/>run · record · scorecard"]
    MCP["evidra-mcp<br/>MCP Server for AI Agents"]
    CI["CI Systems<br/>GitHub Actions · GitLab CI"]
  end
  subgraph Backend ["evidra-api (Self-Hosted)"]
    API["REST API<br/>15 endpoints"]
    DB[("PostgreSQL")]
    API --> DB
  end
  CLI -->|"forward"| API
  MCP -->|"forward"| API
  CI --> CLI
  CLI --> LS[("Local Evidence<br/>append-only JSONL")]
  MCP --> LS`;

const SEQUENCE_CHART = `sequenceDiagram
  participant Agent as AI Agent / CI
  participant CLI as evidra CLI / MCP
  participant Canon as Canonicalize
  participant Risk as Risk Engine
  participant Chain as Evidence Chain
  participant Signal as Signal Detectors
  participant Score as Scoring Engine

  Agent->>CLI: prescribe(tool, operation, artifact)
  CLI->>Canon: SelectAdapter → Normalize
  Canon-->>CLI: CanonicalAction + digests
  CLI->>Risk: RiskLevel(op_class, scope_class)
  Risk-->>CLI: risk_level + risk_tags
  CLI->>Chain: append(prescription entry)
  CLI-->>Agent: prescription_id, risk_level

  Note over Agent: Execute infrastructure operation

  Agent->>CLI: report(prescription_id, exit_code)
  CLI->>Chain: append(report entry, linked)
  CLI->>Signal: detect(entries)
  Signal-->>CLI: signals[]
  CLI-->>Agent: report_id, signals

  Agent->>CLI: scorecard(filters)
  CLI->>Score: compute(entries, weights)
  Score-->>CLI: score, band, confidence
  CLI-->>Agent: scorecard + band`;

const INSTALL_BINARY = `# Download latest release (Linux/macOS)
curl -fsSL https://github.com/samebits/evidra/releases/latest/download/evidra_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz \\
  | tar -xz -C /usr/local/bin evidra

# Run your first observation
evidra run -- kubectl apply -f deploy.yaml

# View the scorecard
evidra scorecard`;

const INSTALL_BREW = `# Install via Homebrew
brew install samebits/tap/evidra

# Run your first observation
evidra run -- kubectl apply -f deploy.yaml

# View the scorecard
evidra scorecard`;

const INSTALL_SELFHOST = `# Download docker-compose.yml
curl -O https://raw.githubusercontent.com/vitas/evidra/main/docker-compose.yml

# Set your API key and start
export EVIDRA_API_KEY=my-secret-key
docker compose up -d

# Verify it's running
curl http://localhost:8080/healthz`;

const SIGNALS = [
  { name: "protocol_violation", weight: "0.35", desc: "Missing prescribe or report, duplicate reports", icon: "\u26A0" },
  { name: "artifact_drift", weight: "0.30", desc: "Artifact content changed between prescribe and execution", icon: "\u21C5" },
  { name: "retry_loop", weight: "0.20", desc: "Same operation retried multiple times after failure", icon: "\u21BA" },
  { name: "thrashing", weight: "0.15", desc: "Rapid apply/delete cycles on the same resources", icon: "\u21AF" },
  { name: "blast_radius", weight: "0.10", desc: "Operations affecting many resources or critical scopes", icon: "\u25C9" },
  { name: "risk_escalation", weight: "0.10", desc: "Actor's operations exceed their baseline risk level", icon: "\u2191" },
  { name: "new_scope", weight: "0.05", desc: "Actor operating in an environment they haven't used before", icon: "\u2737" },
  { name: "repair_loop", weight: "\u22120.05", desc: "Delete-then-recreate patterns \u2014 penalty reduction (positive signal)", icon: "\u2795" },
];

const FEATURES = [
  { icon: "\u25CE", title: "Observe", desc: "Record every infrastructure operation as signed evidence in an append-only, hash-linked chain." },
  { icon: "\u25A4", title: "Measure", desc: "Detect 8 behavioral signals: retry loops, artifact drift, protocol violations, blast radius, and more." },
  { icon: "\u2605", title: "Score", desc: "Compute reliability scorecards with weighted penalties, safety floors, and confidence levels." },
  { icon: "\u21C4", title: "Compare", desc: "Compare actors, tools, and scopes side-by-side with workload overlap analysis." },
];

const GUIDES = [
  { tag: "AI Agents", title: "MCP Setup", desc: "Connect Claude, Cursor, Codex, Gemini, or any MCP-capable agent to Evidra for automatic reliability tracking.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/mcp-setup.md" },
  { tag: "CI / CD", title: "CI Integration", desc: "Add reliability scoring to your CI pipeline \u2014 Terraform, Kubernetes, Docker, and more.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/terraform-ci-quickstart.md" },
  { tag: "Observability", title: "Metrics Export", desc: "Export signals and scores to Grafana, Datadog, or any OTLP-compatible backend.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/observability-quickstart.md" },
  { tag: "Scanners", title: "SARIF Integration", desc: "Ingest findings from Trivy, Kubescape, or any SARIF-compatible security scanner.", href: "https://github.com/vitas/evidra/blob/main/docs/integrations/SCANNER_SARIF_QUICKSTART.md" },
];

type EditorTab = "claude-code" | "json-config" | "codex" | "gemini";

const EDITOR_TABS: { id: EditorTab; label: string }[] = [
  { id: "claude-code", label: "Claude Code" },
  { id: "json-config", label: "Cursor / Claude Desktop" },
  { id: "codex", label: "Codex" },
  { id: "gemini", label: "Gemini CLI" },
];

function mcpConfig(editor: EditorTab, withApi: boolean): string {
  if (editor === "claude-code") {
    if (withApi) {
      return `claude mcp add evidra -- evidra-mcp \\
  --signing-mode optional \\
  --url http://localhost:8080 \\
  --api-key YOUR_KEY \\
  --fallback-offline`;
    }
    return `claude mcp add evidra -- evidra-mcp --signing-mode optional`;
  }
  if (editor === "codex") {
    if (withApi) {
      return `# ~/.codex/config.toml
[mcp_servers.evidra]
command = "evidra-mcp"
args = ["--signing-mode", "optional"]

[mcp_servers.evidra.env]
EVIDRA_URL = "http://localhost:8080"
EVIDRA_API_KEY = "YOUR_KEY"
EVIDRA_FALLBACK = "offline"`;
    }
    return `# ~/.codex/config.toml
[mcp_servers.evidra]
command = "evidra-mcp"
args = ["--signing-mode", "optional"]`;
  }
  if (editor === "gemini") {
    if (withApi) {
      return `// ~/.gemini/settings.json
{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--signing-mode", "optional"],
      "env": {
        "EVIDRA_URL": "http://localhost:8080",
        "EVIDRA_API_KEY": "YOUR_KEY",
        "EVIDRA_FALLBACK": "offline"
      }
    }
  }
}`;
    }
    return `// ~/.gemini/settings.json
{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--signing-mode", "optional"]
    }
  }
}`;
  }
  // json-config (Cursor, Claude Desktop, Windsurf)
  if (withApi) {
    return `{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--signing-mode", "optional"],
      "env": {
        "EVIDRA_URL": "http://localhost:8080",
        "EVIDRA_API_KEY": "YOUR_KEY",
        "EVIDRA_FALLBACK": "offline"
      }
    }
  }
}`;
  }
  return `{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--signing-mode", "optional"]
    }
  }
}`;
}

export function Landing() {
  return (
    <>
      <Hero />
      <Divider />
      <Features />
      <Divider />
      <Signals />
      <Divider />
      <Architecture />
      <Divider />
      <GettingStarted />
      <Divider />
      <McpSetup />
      <Divider />
      <ApiReference />
      <Divider />
      <GuidesSection />
    </>
  );
}

function Divider() {
  return <hr className="h-px border-none bg-[linear-gradient(90deg,transparent,var(--color-accent-tint),var(--color-accent),var(--color-accent-tint),transparent)] m-0" />;
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <div className="font-mono text-[0.72rem] font-medium tracking-widest uppercase text-accent mb-3">{children}</div>;
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h2 className="text-[1.28rem] font-bold text-fg tracking-tight mb-2">{children}</h2>;
}

function Container({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <div className={`max-w-[980px] mx-auto px-8 ${className}`}>{children}</div>;
}

function Hero() {
  return (
    <section className="relative py-24 pb-20 text-center bg-[radial-gradient(ellipse_80%_60%_at_50%_0%,var(--color-accent-subtle),var(--color-bg)_70%)] overflow-hidden">
      <div className="absolute inset-0 bg-[radial-gradient(circle,var(--color-accent)_1px,transparent_1px)] bg-[length:24px_24px] opacity-[0.06] [mask-image:radial-gradient(ellipse_60%_70%_at_50%_30%,black,transparent)]" />
      <Container className="relative">
        <div className="inline-flex items-center gap-2 font-mono text-[0.75rem] font-medium text-accent bg-accent-subtle border border-border rounded-full px-4 py-1 mb-6 tracking-wide">
          <span className="w-1.5 h-1.5 rounded-full bg-accent inline-block animate-pulse" />
          Open Source &middot; Apache 2.0
        </div>
        <h1 className="text-[clamp(2.2rem,5vw,3.2rem)] font-extrabold text-fg leading-[1.15] tracking-tighter mb-5">
          Reliability Scoring for<br />Infrastructure Automation
        </h1>
        <p className="text-[1.15rem] text-fg-muted max-w-[640px] mx-auto mb-3 leading-relaxed">
          Evidra measures operational reliability across CI pipelines, IaC workflows, and AI agents &mdash; by recording evidence, computing behavioral signals, and producing scorecards.
        </p>
        <p className="text-[0.92rem] text-fg-muted max-w-[580px] mx-auto mb-10 leading-relaxed opacity-80">
          Detect retry loops, artifact drift, and protocol violations. Reliability scoring for CI/CD, Terraform, Kubernetes, and AI agents.
        </p>
        <div className="flex gap-3 justify-center flex-wrap">
          <a href="#get-started" className="btn-primary inline-flex items-center gap-1.5 px-5 py-2.5 rounded-lg text-[0.88rem] font-semibold bg-accent text-white transition-all hover:bg-accent-bright hover:-translate-y-0.5 hover:shadow-lg no-underline">
            Get Started
          </a>
          <a href="/docs/api" className="inline-flex items-center gap-1.5 px-5 py-2.5 rounded-lg text-[0.88rem] font-semibold bg-transparent border border-border text-fg-muted transition-all hover:border-accent hover:text-fg no-underline">
            API Docs
          </a>
        </div>
      </Container>
    </section>
  );
}

function Features() {
  return (
    <section id="features" className="py-14">
      <Container>
        <SectionLabel>Capabilities</SectionLabel>
        <SectionTitle>What It Does</SectionTitle>
        <p className="text-fg-muted mb-10 text-[1.14rem]">A flight recorder for infrastructure automation &mdash; observe, measure, score, compare.</p>
        <div className="grid grid-cols-4 gap-5 max-md:grid-cols-2 max-sm:grid-cols-1">
          {FEATURES.map((f) => (
            <div key={f.title} className="bg-bg-elevated border border-border border-l-[3px] border-l-accent rounded-lg p-6 shadow-[var(--shadow-card)] transition-all hover:shadow-[var(--shadow-card-lg)] hover:-translate-y-0.5">
              <div className="w-9 h-9 rounded-lg bg-accent-subtle border border-border flex items-center justify-center text-lg mb-4">{f.icon}</div>
              <h3 className="text-[0.92rem] font-bold text-fg mb-1.5">{f.title}</h3>
              <p className="text-[0.83rem] text-fg-muted leading-relaxed">{f.desc}</p>
            </div>
          ))}
        </div>
      </Container>
    </section>
  );
}

function Signals() {
  return (
    <section id="signals" className="py-14 bg-bg-alt">
      <Container>
        <SectionLabel>Behavioral Signals</SectionLabel>
        <SectionTitle>8 Signals, Weighted Scoring</SectionTitle>
        <p className="text-fg-muted mb-8 text-[1.14rem]">Each signal detects a distinct operational anti-pattern. Weights reflect severity &mdash; higher weight means greater impact on the reliability score.</p>
        <div className="grid grid-cols-1 gap-[1px] bg-border rounded-[10px] overflow-hidden shadow-[var(--shadow-card)]">
          {SIGNALS.map((s) => (
            <div key={s.name} className="bg-bg-elevated flex items-center gap-5 px-6 py-4 max-sm:flex-wrap">
              <div className="w-8 h-8 rounded-lg bg-accent-subtle border border-border flex items-center justify-center text-base shrink-0">{s.icon}</div>
              <div className="font-mono text-[0.82rem] font-semibold text-fg w-[170px] shrink-0">{s.name}</div>
              <div className="text-[0.83rem] text-fg-muted leading-relaxed flex-1">{s.desc}</div>
              <div className={`font-mono text-[0.78rem] font-medium shrink-0 tabular-nums ${s.weight.startsWith("\u2212") ? "text-green-500" : "text-accent"}`}>{s.weight}</div>
            </div>
          ))}
        </div>
        <p className="text-[0.82rem] text-fg-muted mt-4 text-center">
          Score = 100 &times; (1 &minus; weighted penalty). Range 0&ndash;100. Bands: excellent &ge; 90, good &ge; 75, fair &ge; 50, poor &ge; 25, critical &lt; 25.
        </p>
      </Container>
    </section>
  );
}

function Architecture() {
  const [tab, setTab] = useState<"pipeline" | "system" | "sequence">("pipeline");
  return (
    <section id="architecture" className="py-14 bg-bg-alt">
      <Container>
        <SectionLabel>Architecture</SectionLabel>
        <SectionTitle>How It All Fits Together</SectionTitle>
        <p className="text-fg-muted mb-10 text-[1.14rem]">Three views: scoring pipeline, system deployment, and the protocol sequence.</p>
        <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px] mb-6">
          <TabBtn active={tab === "pipeline"} onClick={() => setTab("pipeline")}>How It Works</TabBtn>
          <TabBtn active={tab === "sequence"} onClick={() => setTab("sequence")}>Protocol Sequence</TabBtn>
          <TabBtn active={tab === "system"} onClick={() => setTab("system")}>System Architecture</TabBtn>
        </div>
        {tab === "pipeline" && <MermaidDiagram chart={PIPELINE_CHART} />}
        {tab === "system" && <MermaidDiagram chart={SYSTEM_CHART} />}
        {tab === "sequence" && <MermaidDiagram chart={SEQUENCE_CHART} />}
      </Container>
    </section>
  );
}

function GettingStarted() {
  const [tab, setTab] = useState<"binary" | "brew" | "selfhost">("binary");
  const code = tab === "binary" ? INSTALL_BINARY : tab === "brew" ? INSTALL_BREW : INSTALL_SELFHOST;
  return (
    <section id="get-started" className="py-14">
      <Container>
        <SectionLabel>Quick Start</SectionLabel>
        <SectionTitle>Getting Started</SectionTitle>
        <p className="text-fg-muted mb-10 text-[1.14rem]">Up and running in under 5 minutes.</p>
        <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px] mb-6">
          <TabBtn active={tab === "binary"} onClick={() => setTab("binary")}>Binary</TabBtn>
          <TabBtn active={tab === "brew"} onClick={() => setTab("brew")}>Homebrew</TabBtn>
          <TabBtn active={tab === "selfhost"} onClick={() => setTab("selfhost")}>Self-Hosted</TabBtn>
        </div>
        <CodeBlock code={code} />
      </Container>
    </section>
  );
}

function McpSetup() {
  const [editor, setEditor] = useState<EditorTab>("claude-code");
  const [withApi, setWithApi] = useState(false);
  const code = mcpConfig(editor, withApi);

  const configPathNote = editor === "json-config"
    ? "Add to .cursor/mcp.json (Cursor), claude_desktop_config.json (Claude Desktop), or ~/.codeium/windsurf/mcp_config.json (Windsurf)."
    : editor === "codex" ? "Edit ~/.codex/config.toml."
    : editor === "gemini" ? "Edit ~/.gemini/settings.json."
    : "Run in your terminal.";

  return (
    <section id="mcp-setup" className="py-14 bg-bg-alt">
      <Container>
        <SectionLabel>AI Agents</SectionLabel>
        <SectionTitle>MCP Server Setup</SectionTitle>
        <p className="text-fg-muted mb-6 text-[1.14rem]">Connect any MCP-capable AI agent to Evidra for automatic reliability tracking.</p>

        <div className="mb-8">
          <h3 className="text-[0.95rem] font-bold text-fg mb-3">1. Install</h3>
          <CodeBlock code="brew install samebits/tap/evidra" />
          <p className="text-[0.83rem] text-fg-muted mt-2">
            Or: <code className="text-[0.8rem]">go install samebits.com/evidra-benchmark/cmd/evidra-mcp@latest</code>
          </p>
        </div>

        <div className="mb-8">
          <h3 className="text-[0.95rem] font-bold text-fg mb-3">2. Connect to your editor</h3>

          <div className="flex items-center gap-4 mb-4 flex-wrap">
            <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px]">
              {EDITOR_TABS.map((tab) => (
                <TabBtn key={tab.id} active={editor === tab.id} onClick={() => setEditor(tab.id)}>{tab.label}</TabBtn>
              ))}
            </div>
            <label className="flex items-center gap-2 text-[0.82rem] text-fg-muted cursor-pointer select-none">
              <input
                type="checkbox"
                checked={withApi}
                onChange={(e) => setWithApi(e.target.checked)}
                className="accent-accent"
              />
              Forward to API server
            </label>
          </div>

          <p className="text-[0.83rem] text-fg-muted mb-3">{configPathNote}</p>
          <CodeBlock code={code} />
        </div>

        <div className="mb-8">
          <h3 className="text-[0.95rem] font-bold text-fg mb-3">3. Verify</h3>
          <p className="text-[0.85rem] text-fg-muted">
            Restart your editor. Ask your agent: <em>&ldquo;What tools do you have from Evidra?&rdquo;</em> &mdash; you should see <code>prescribe</code>, <code>report</code>, and <code>get_event</code>.
          </p>
        </div>

        <div className="bg-bg-elevated border border-border rounded-[10px] p-6 shadow-[var(--shadow-card)]">
          <h3 className="text-[0.92rem] font-bold text-fg mb-2">How it works</h3>
          <p className="text-[0.83rem] text-fg-muted leading-relaxed mb-3">
            The MCP server exposes three tools. Your agent calls <code>prescribe</code> before every infrastructure mutation (apply, delete, upgrade) and <code>report</code> after execution with the exit code. Evidra records the evidence, detects behavioral signals, and produces reliability scores.
          </p>
          <div className="grid grid-cols-3 gap-4 max-sm:grid-cols-1">
            <div className="text-center">
              <div className="font-mono text-[0.78rem] font-semibold text-accent mb-1">prescribe</div>
              <div className="text-[0.78rem] text-fg-muted">Record intent before execution</div>
            </div>
            <div className="text-center">
              <div className="font-mono text-[0.78rem] font-semibold text-accent mb-1">report</div>
              <div className="text-[0.78rem] text-fg-muted">Record outcome after execution</div>
            </div>
            <div className="text-center">
              <div className="font-mono text-[0.78rem] font-semibold text-accent mb-1">get_event</div>
              <div className="text-[0.78rem] text-fg-muted">Look up evidence for audit</div>
            </div>
          </div>
        </div>

        <p className="text-[0.85rem] text-fg-muted mt-6">
          Full setup guide with agent instructions, configuration options, and troubleshooting:{" "}
          <a href="https://github.com/vitas/evidra/blob/main/docs/guides/mcp-setup.md" target="_blank" rel="noopener" className="font-semibold">MCP Setup Guide &rarr;</a>
        </p>
      </Container>
    </section>
  );
}

function ApiReference() {
  return (
    <section id="api" className="py-14 bg-bg-alt">
      <Container>
        <SectionLabel>API</SectionLabel>
        <SectionTitle>API Reference</SectionTitle>
        <p className="text-fg-muted mb-10 text-[1.14rem]">Full OpenAPI 3.0 documentation for all endpoints.</p>
        <a href="/docs/api" className="flex items-center justify-between bg-bg-elevated border border-border rounded-[10px] p-6 px-8 shadow-[var(--shadow-card)] transition-all hover:shadow-[var(--shadow-card-lg)] hover:border-accent no-underline">
          <div>
            <h3 className="text-base text-fg mb-1">Interactive API Documentation</h3>
            <p className="text-[0.85rem] text-fg-muted">Explore all 15 endpoints with request/response schemas, authentication details, and examples.</p>
          </div>
          <div className="font-mono text-[0.8rem] text-accent font-medium whitespace-nowrap">/docs/api &rarr;</div>
        </a>
      </Container>
    </section>
  );
}

function GuidesSection() {
  return (
    <section id="guides" className="py-14">
      <Container>
        <SectionLabel>Guides</SectionLabel>
        <SectionTitle>Integrate Into Your Workflow</SectionTitle>
        <p className="text-fg-muted mb-10 text-[1.14rem]">Step-by-step guides for common integration patterns.</p>
        <div className="grid grid-cols-4 gap-5 max-lg:grid-cols-2 max-sm:grid-cols-1">
          {GUIDES.map((g) => (
            <a key={g.title} href={g.href} target="_blank" rel="noopener" className="bg-bg-elevated border border-border rounded-[10px] p-6 shadow-[var(--shadow-card)] transition-all hover:shadow-[var(--shadow-card-lg)] hover:border-accent hover:-translate-y-0.5 no-underline block">
              <div className="font-mono text-[0.7rem] font-medium text-accent tracking-wide uppercase mb-2">{g.tag}</div>
              <h3 className="text-[0.95rem] text-fg mb-1.5 font-semibold">{g.title}</h3>
              <p className="text-[0.83rem] text-fg-muted leading-relaxed">{g.desc}</p>
            </a>
          ))}
        </div>
        <div className="mt-8 py-4 px-6 bg-accent-subtle border border-border rounded-lg text-center text-[0.88rem] text-fg-muted">
          More guides, GitHub Actions setup, architecture docs, and source code on{" "}
          <a href="https://github.com/vitas/evidra" target="_blank" rel="noopener" className="font-semibold">GitHub &rarr;</a>
        </div>
      </Container>
    </section>
  );
}

function TabBtn({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`border-none rounded-md px-4 py-2 cursor-pointer text-[0.82rem] font-semibold font-sans transition-all ${
        active ? "bg-bg-elevated text-fg shadow-[var(--shadow-card)]" : "bg-transparent text-fg-muted hover:text-fg"
      }`}
    >
      {children}
    </button>
  );
}
