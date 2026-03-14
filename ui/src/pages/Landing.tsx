import { useState } from "react";
import { Link } from "react-router";
import { CodeBlock } from "../components/CodeBlock";
import { MermaidDiagram } from "../components/MermaidDiagram";

const PIPELINE_CHART = `flowchart LR
  A["Raw Artifact"] --> B{"Adapter<br/>Selection"}
  B -->|K8s| C1["K8s Adapter"]
  B -->|Terraform| C2["TF Adapter"]
  B -->|Docker| C3["Docker Adapter"]
  B -->|Other| C4["Generic Adapter"]
  C1 & C2 & C3 & C4 --> D["CanonicalAction"]
  D --> E["Risk Assembly<br/>native or matrix + findings"]
  E --> F["Prescription<br/>risk inputs + effective risk"]
  F --> G[("Evidence<br/>Chain")]
  H1["Execution outcome<br/>verdict + exit_code"] --> I["Report"]
  H2["Deliberate refusal<br/>verdict=declined + decision_context"] --> I
  I --> G
  G --> J["Signal Detectors<br/>8 signals"]
  J --> K["Scoring Engine"]
  K --> L["Scorecard<br/>+ Band"]`;

const SYSTEM_CHART = `flowchart TB
  subgraph Clients ["Client Layer"]
    CI["CI / AI Agents<br/>GitHub Actions · Codex · Claude"]
    CLI["evidra CLI<br/>record · import · scorecard"]
    MCP["evidra-mcp<br/>MCP Server for AI Agents"]
    Hooks["ArgoCD / generic webhooks"]
  end
  subgraph Local ["Local Path"]
    LS[("Local Evidence<br/>append-only JSONL")]
  end
  subgraph Backend ["evidra-api<br/>Self-Hosted"]
    API["REST API<br/>ingest · browse · scorecard · explain"]
    DB[("PostgreSQL<br/>tenant evidence store")]
    API --> DB
  end
  subgraph Analytics ["Reliability Analytics"]
    Engine["Shared Signal +<br/>Scoring Engine"]
  end
  CI --> CLI
  CI --> MCP
  CLI -->|"append evidence"| LS
  MCP -->|"append evidence"| LS
  CLI -->|"forward"| API
  MCP -->|"forward"| API
  Hooks -->|"webhook ingestion"| API
  LS --> Engine
  DB --> Engine`;

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
  CLI->>Risk: build risk inputs
  Risk-->>CLI: risk inputs + effective risk
  CLI->>Chain: append(prescription entry)
  CLI-->>Agent: prescription id + effective risk

  alt Agent executes infrastructure operation
    Note over Agent: Execute infrastructure mutation
    Agent->>CLI: report(prescription id, verdict, exit code)
  else Agent denies or deliberately refuses
    Note over Agent: Record the deny decision explicitly
    Agent->>CLI: report(prescription id, declined, decision context)
  end
  CLI->>Chain: append(report entry, linked)
  CLI->>Signal: detect patterns
  Signal-->>CLI: signal_summary + confidence
  CLI-->>Agent: report id + score band

  Agent->>CLI: scorecard(filters)
  CLI->>Score: compute(entries, scoring profile)
  Score-->>CLI: score, band, confidence
  CLI-->>Agent: scorecard + band`;

const INSTALL_BINARY = `# Download latest release (Linux/macOS)
curl -fsSL https://github.com/samebits/evidra/releases/latest/download/evidra_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz \\
  | tar -xz -C /usr/local/bin evidra

# Run your first observation
evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml

# View the scorecard
evidra scorecard`;

const INSTALL_BREW = `# Install via Homebrew
brew install samebits/tap/evidra

# Run your first observation
evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml

# View the scorecard
evidra scorecard`;

const INSTALL_SELFHOST = `# Download docker-compose.yml
curl -O https://raw.githubusercontent.com/vitas/evidra/main/docker-compose.yml

# Set your API key and start
export EVIDRA_API_KEY=my-secret-key
docker compose up -d

# Verify it's running
curl http://localhost:8080/healthz

# Query tenant-wide hosted analytics
curl -H "Authorization: Bearer $EVIDRA_API_KEY" \\
  "http://localhost:8080/v1/evidence/scorecard?period=30d"`;

const PRIMARY_SIGNALS = [
  {
    name: "protocol_violation",
    icon: "\u26A0",
    desc: "Prescribe without report \u2014 agent crashed, timed out, or skipped the protocol. Report without prescribe \u2014 unauthorized action. The most operationally immediate signal.",
    tag: "fires immediately",
  },
  {
    name: "retry_loop",
    icon: "\u21BA",
    desc: "Same intent retried 3+ times after failure within 30 minutes. Your agent is stuck in a loop.",
    tag: "fires immediately",
  },
  {
    name: "blast_radius",
    icon: "\u25C9",
    desc: "Destroy operation affecting more than 5 resources. High-impact deletion that warrants review.",
    tag: "fires immediately",
  },
];

const FEATURES = [
  { icon: "\u25CE", title: "Prescribe", desc: "Before kubectl runs, the evidence already exists. Record the artifact, its canonical form, the full risk_inputs panel, and the rolled-up effective_risk \u2014 at the moment of intent." },
  { icon: "\u25A4", title: "Report", desc: "Record the terminal outcome \u2014 success, failure, or an explicit refusal with structured context. Every prescribe gets exactly one report. No silent gaps." },
  { icon: "\u2605", title: "Evidence", desc: "Signed, timestamped, hash-chained. The evidence chain is append-only and tamper-evident. Cryptographically verifiable by anyone, editable by no one." },
  { icon: "\u21C4", title: "Detect", desc: "The protocol structure makes behavioral patterns visible: agents stuck in retry loops, broken prescribe/report pairs, high-impact deletions. Reliability scorecards across actors, sessions, and time." },
];

const GUIDES = [
  { tag: "AI Agents", title: "MCP Setup", desc: "Connect Claude Code, Cursor, Codex, Gemini, or any MCP agent to the prescribe/report protocol.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/mcp-setup.md" },
  { tag: "AI Agents", title: "Skill Setup", desc: "Install the Evidra skill \u2014 agents with the skill achieve 100% protocol compliance for infrastructure mutations.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/skill-setup.md" },
  { tag: "Platform", title: "Self-Hosted Setup", desc: "Centralize evidence across agents and pipelines. Compare agent reliability fleet-wide.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/self-hosted-setup.md" },
  { tag: "CI / CD", title: "Pipeline Setup", desc: "Add prescribe/report to your CI pipeline. Record intent before deploy, outcome after. Works without MCP \u2014 the CLI wraps any command.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/terraform-ci-quickstart.md" },
  { tag: "Observability", title: "Metrics Export", desc: "Export signals and scores to Grafana, Datadog, or any OTLP-compatible backend.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/observability-quickstart.md" },
];

type EditorTab = "claude-code" | "json-config" | "codex" | "gemini";

const EDITOR_TABS: { id: EditorTab; label: string }[] = [
  { id: "claude-code", label: "Claude Code" },
  { id: "json-config", label: "Cursor / Claude Desktop" },
  { id: "codex", label: "Codex" },
  { id: "gemini", label: "Gemini CLI" },
];

type LandingConfigMode = "hosted" | "self-hosted" | "local";

const HOSTED_MCP_URL = "https://evidra.cc/mcp";

function mcpConfig(editor: EditorTab, mode: LandingConfigMode): string {
  if (mode === "hosted") {
    const url = HOSTED_MCP_URL;

    if (editor === "claude-code") {
      return `claude mcp add --transport http \\
  -H "Authorization: Bearer YOUR_KEY" \\
  -s user evidra ${url}`;
    }

    if (editor === "codex") {
      return `# ~/.codex/config.toml
[mcp_servers.evidra]
url = "${url}"

[mcp_servers.evidra.headers]
Authorization = "Bearer YOUR_KEY"`;
    }

    const jsonObj = {
      mcpServers: {
        evidra: {
          url,
          headers: { Authorization: "Bearer YOUR_KEY" },
        },
      },
    };

    if (editor === "gemini") {
      return `// ~/.gemini/settings.json\n${JSON.stringify(jsonObj, null, 2)}`;
    }
    return JSON.stringify(jsonObj, null, 2);
  }

  if (mode === "self-hosted") {
    if (editor === "claude-code") {
      return `claude mcp add evidra -- evidra-mcp \\
  --signing-mode optional \\
  --url http://localhost:8080 \\
  --api-key YOUR_KEY \\
  --fallback-offline`;
    }

    if (editor === "codex") {
      return `# ~/.codex/config.toml
[mcp_servers.evidra]
command = "evidra-mcp"
args = ["--signing-mode", "optional"]

[mcp_servers.evidra.env]
EVIDRA_URL = "http://localhost:8080"
EVIDRA_API_KEY = "YOUR_KEY"
EVIDRA_FALLBACK = "offline"`;
    }

    const jsonObj = {
      mcpServers: {
        evidra: {
          command: "evidra-mcp",
          args: ["--signing-mode", "optional"],
          env: {
            EVIDRA_URL: "http://localhost:8080",
            EVIDRA_API_KEY: "YOUR_KEY",
            EVIDRA_FALLBACK: "offline",
          },
        },
      },
    };

    if (editor === "gemini") {
      return `// ~/.gemini/settings.json\n${JSON.stringify(jsonObj, null, 2)}`;
    }
    return JSON.stringify(jsonObj, null, 2);
  }

  // Local mode
  if (editor === "claude-code") {
    return `claude mcp add evidra -- evidra-mcp --signing-mode optional`;
  }

  if (editor === "codex") {
    return `# ~/.codex/config.toml
[mcp_servers.evidra]
command = "evidra-mcp"
args = ["--signing-mode", "optional"]`;
  }

  const jsonObj = {
    mcpServers: {
      evidra: {
        command: "evidra-mcp",
        args: ["--signing-mode", "optional"],
      },
    },
  };

  if (editor === "gemini") {
    return `// ~/.gemini/settings.json\n${JSON.stringify(jsonObj, null, 2)}`;
  }
  return JSON.stringify(jsonObj, null, 2);
}

export function Landing() {
  return (
    <>
      <Hero />
      <Divider />
      <TheGap />
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
          Open Source &middot; Apache 2.0 &middot; Flight Recorder for AI Infrastructure Agents
        </div>
        <h1 className="text-[clamp(2.2rem,5vw,3.2rem)] font-extrabold text-fg leading-[1.15] tracking-tighter mb-5">
          Know what your agent intended.<br />Know what actually happened.
        </h1>
        <p className="text-[1.15rem] text-fg-muted max-w-[640px] mx-auto mb-3 leading-relaxed">
          The prescribe/report protocol for AI infrastructure agents.
        </p>
        <p className="text-[0.92rem] text-fg-muted max-w-[580px] mx-auto mb-10 leading-relaxed opacity-80">
          Evidra records intent before execution and outcome after &mdash; in a signed evidence chain that no one can edit after the fact. When your agent decides not to act, that decision is recorded too.
        </p>
        <div className="flex gap-3 justify-center flex-wrap">
          <a href="#get-started" className="btn-primary inline-flex items-center gap-1.5 px-5 py-2.5 rounded-lg text-[0.88rem] font-semibold bg-accent text-white transition-all hover:bg-accent-bright hover:-translate-y-0.5 hover:shadow-lg no-underline">
            Quick Start
          </a>
          <Link to="/onboarding" className="inline-flex items-center gap-1.5 px-5 py-2.5 rounded-lg text-[0.88rem] font-semibold bg-transparent border border-border text-fg-muted transition-all hover:border-accent hover:text-fg no-underline">
            Get API Key
          </Link>
          <a href="/docs/api" className="inline-flex items-center gap-1.5 px-5 py-2.5 rounded-lg text-[0.88rem] font-semibold bg-transparent border border-border text-fg-muted transition-all hover:border-accent hover:text-fg no-underline">
            API Docs
          </a>
        </div>
      </Container>
    </section>
  );
}

function TheGap() {
  const columns = [
    {
      icon: "\u25B6",
      title: "What was the agent trying to do?",
      body: "Your agent decides at runtime which YAML to generate, which namespace to target, which command to run. That intent exists for one moment \u2014 then it\u2019s gone. OTel, Datadog, and CloudTrail only see what happened after.",
    },
    {
      icon: "\u2718",
      title: "Did the agent decide not to act \u2014 and why?",
      body: "When an agent skips a risky operation, nothing is recorded. No log entry. No audit trail. The safest decision your agent makes is the one that\u2019s completely invisible.",
    },
    {
      icon: "\u2194",
      title: "Did the outcome match the intent?",
      body: "The agent prescribed one artifact. Did it apply the same one? Without recording intent before execution, drift between plan and action is undetectable.",
    },
  ];

  return (
    <section className="py-14 bg-bg-alt">
      <Container>
        <SectionLabel>The Problem</SectionLabel>
        <SectionTitle>AI Agents Make Decisions. Nothing Records Them.</SectionTitle>
        <div className="grid grid-cols-3 gap-5 mt-10 max-md:grid-cols-1">
          {columns.map((c) => (
            <div key={c.title} className="bg-bg-elevated border border-border rounded-lg p-6 shadow-[var(--shadow-card)]">
              <div className="w-9 h-9 rounded-lg bg-accent-subtle border border-border flex items-center justify-center text-lg mb-4">{c.icon}</div>
              <h3 className="text-[0.92rem] font-bold text-fg mb-2">{c.title}</h3>
              <p className="text-[0.83rem] text-fg-muted leading-relaxed">{c.body}</p>
            </div>
          ))}
        </div>
      </Container>
    </section>
  );
}

function Features() {
  return (
    <section id="features" className="py-14">
      <Container>
        <SectionLabel>The Protocol</SectionLabel>
        <SectionTitle>Prescribe Before. Report After. Evidence Always.</SectionTitle>
        <p className="text-fg-muted mb-10 text-[1.14rem]">Every infrastructure mutation follows the same lifecycle. The agent records what it intends to do, does it (or refuses), and records what happened. Evidra stores the evidence.</p>
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
        <SectionLabel>Behavioral Detection</SectionLabel>
        <SectionTitle>Patterns That Fire on Day One</SectionTitle>
        <p className="text-fg-muted mb-8 text-[1.14rem]">
          The prescribe/report structure makes agent behavior patterns visible without external instrumentation. Three signals fire immediately in real operations.
        </p>
        <div className="grid grid-cols-3 gap-5 mb-6 max-md:grid-cols-1">
          {PRIMARY_SIGNALS.map((s) => (
            <div key={s.name} className="bg-bg-elevated border border-border border-l-[3px] border-l-accent rounded-lg p-6 shadow-[var(--shadow-card)]">
              <div className="flex items-center justify-between mb-4">
                <div className="w-9 h-9 rounded-lg bg-accent-subtle border border-border flex items-center justify-center text-lg">{s.icon}</div>
                <span className="font-mono text-[0.7rem] font-medium text-accent tracking-wide uppercase">{s.tag}</span>
              </div>
              <div className="font-mono text-[0.82rem] font-semibold text-fg mb-2">{s.name}</div>
              <p className="text-[0.83rem] text-fg-muted leading-relaxed">{s.desc}</p>
            </div>
          ))}
        </div>
        <div className="bg-bg-elevated border border-border rounded-[10px] p-5 px-6 shadow-[var(--shadow-card)]">
          <p className="text-[0.83rem] text-fg-muted leading-relaxed">
            Additional signals &mdash; <code>artifact_drift</code>, <code>new_scope</code>, <code>repair_loop</code>, <code>thrashing</code>, <code>risk_escalation</code> &mdash; contribute to scoring and mature as evidence accumulates. All eight are documented in the{" "}
            <a href="https://github.com/vitas/evidra/blob/main/docs/signal-spec.md" target="_blank" rel="noopener" className="font-semibold">Signal Specification &rarr;</a>
          </p>
        </div>
        <p className="text-[0.82rem] text-fg-muted mt-4 text-center">
          Score = 100 &times; (1 &minus; weighted penalty). Bands: excellent &ge; 99, good &ge; 95, fair &ge; 90, poor &lt; 90.
        </p>
      </Container>
    </section>
  );
}

function Architecture() {
  const [tab, setTab] = useState<"pipeline" | "system" | "sequence">("sequence");
  return (
    <section id="architecture" className="py-14 bg-bg-alt">
      <Container>
        <SectionLabel>Architecture</SectionLabel>
        <SectionTitle>From Agent Intent to Signed Evidence</SectionTitle>
        <p className="text-fg-muted mb-10 text-[1.14rem]">Follow one operation through the protocol &mdash; from the moment an agent decides to act, through execution, to fleet-wide analytics.</p>
        <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px] mb-6">
          <TabBtn active={tab === "sequence"} onClick={() => setTab("sequence")}>Protocol Flow</TabBtn>
          <TabBtn active={tab === "system"} onClick={() => setTab("system")}>System Architecture</TabBtn>
          <TabBtn active={tab === "pipeline"} onClick={() => setTab("pipeline")}>Scoring Pipeline</TabBtn>
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
        <p className="text-fg-muted mb-10 text-[1.14rem]">Record your first prescribe/report lifecycle in under 5 minutes. Works with kubectl, helm, terraform, and docker.</p>
        <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px] mb-6">
          <TabBtn active={tab === "binary"} onClick={() => setTab("binary")}>Binary</TabBtn>
          <TabBtn active={tab === "brew"} onClick={() => setTab("brew")}>Homebrew</TabBtn>
          <TabBtn active={tab === "selfhost"} onClick={() => setTab("selfhost")}>Self-Hosted</TabBtn>
        </div>
        <CodeBlock code={code} />
        {tab === "selfhost" && (
          <p className="text-[0.85rem] text-fg-muted mt-4">
            Self-hosted centralizes evidence across agents and pipelines. Compare agent reliability fleet-wide, ingest ArgoCD webhooks, and run team-wide analytics over stored evidence.{" "}
            <a href="https://github.com/vitas/evidra/blob/main/docs/guides/self-hosted-setup.md" target="_blank" rel="noopener" className="font-semibold">Status guide &rarr;</a>
          </p>
        )}
      </Container>
    </section>
  );
}

function McpSetup() {
  const [editor, setEditor] = useState<EditorTab>("claude-code");
  const [mode, setMode] = useState<LandingConfigMode>("hosted");
  const code = mcpConfig(editor, mode);

  const configPathNote = editor === "json-config"
    ? "Add to .cursor/mcp.json (Cursor), claude_desktop_config.json (Claude Desktop), or ~/.codeium/windsurf/mcp_config.json (Windsurf)."
    : editor === "codex" ? "Edit ~/.codex/config.toml."
    : editor === "gemini" ? "Edit ~/.gemini/settings.json."
    : "Run in your terminal.";

  return (
    <section id="mcp-setup" className="py-14 bg-bg-alt">
      <Container>
        <SectionLabel>AI Agents</SectionLabel>
        <SectionTitle>Give Your Agent the Protocol</SectionTitle>
        <p className="text-fg-muted mb-6 text-[1.14rem]">Connect any MCP-capable agent to Evidra. The agent prescribes before every infrastructure mutation and reports the outcome. When it decides not to act, it reports why.</p>

        {mode !== "hosted" && (
          <div className="mb-8">
            <h3 className="text-[0.95rem] font-bold text-fg mb-3">1. Install</h3>
            <CodeBlock code="brew install samebits/tap/evidra" />
            <p className="text-[0.83rem] text-fg-muted mt-2">
              Or: <code className="text-[0.8rem]">go install samebits.com/evidra/cmd/evidra-mcp@latest</code>
            </p>
          </div>
        )}

        <div className="mb-8">
          <h3 className="text-[0.95rem] font-bold text-fg mb-3">{mode === "hosted" ? "1" : "2"}. Connect to your editor</h3>

          <div className="flex items-center gap-4 mb-4 flex-wrap">
            <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px]">
              {EDITOR_TABS.map((tab) => (
                <TabBtn key={tab.id} active={editor === tab.id} onClick={() => setEditor(tab.id)}>{tab.label}</TabBtn>
              ))}
            </div>
            <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px]">
              {(["hosted", "self-hosted", "local"] as LandingConfigMode[]).map((m) => (
                <TabBtn key={m} active={mode === m} onClick={() => setMode(m)}>
                  {m === "hosted" ? "Hosted" : m === "self-hosted" ? "Self-hosted" : "Local only"}
                </TabBtn>
              ))}
            </div>
          </div>

          <p className="text-[0.83rem] text-fg-muted mb-3">{configPathNote}</p>
          <CodeBlock code={code} />
        </div>

        {editor === "claude-code" && mode !== "hosted" && (
          <div className="mb-8">
            <h3 className="text-[0.95rem] font-bold text-fg mb-3">3. Install the Evidra skill</h3>
            <CodeBlock code="evidra skill install" />
            <p className="text-[0.83rem] text-fg-muted mt-2">
              The MCP server gives agents the tools. The skill teaches them <em>when</em> and <em>how</em> to use them &mdash; achieving 100% protocol compliance.{" "}
              <a href="https://github.com/vitas/evidra/blob/main/docs/guides/skill-setup.md" target="_blank" rel="noopener" className="font-semibold">Skill Setup Guide &rarr;</a>
            </p>
          </div>
        )}

        <div className="mb-8">
          <h3 className="text-[0.95rem] font-bold text-fg mb-3">{editor === "claude-code" && mode !== "hosted" ? "4" : mode === "hosted" ? "2" : "3"}. Verify</h3>
          <p className="text-[0.85rem] text-fg-muted">
            Restart your editor. Ask your agent: <em>&ldquo;What tools do you have from Evidra?&rdquo;</em> &mdash; you should see <code>prescribe</code>, <code>report</code>, and <code>get_event</code>.
          </p>
        </div>

        <div className="bg-bg-elevated border border-border rounded-[10px] p-6 shadow-[var(--shadow-card)]">
          <h3 className="text-[0.92rem] font-bold text-fg mb-2">How it works</h3>
          <p className="text-[0.83rem] text-fg-muted leading-relaxed mb-3">
            Every infrastructure mutation follows the same lifecycle. The agent calls <code>prescribe</code> with the artifact before execution &mdash; Evidra returns risk level, risk tags, and a prescription ID. After execution (or refusal), the agent calls <code>report</code> with the outcome. The evidence chain grows. Behavioral patterns become visible.
          </p>
          <div className="grid grid-cols-3 gap-4 max-sm:grid-cols-1">
            <div className="text-center">
              <div className="font-mono text-[0.78rem] font-semibold text-accent mb-1">prescribe</div>
              <div className="text-[0.78rem] text-fg-muted">What does the agent intend to do? Record before execution.</div>
            </div>
            <div className="text-center">
              <div className="font-mono text-[0.78rem] font-semibold text-accent mb-1">report</div>
              <div className="text-[0.78rem] text-fg-muted">What actually happened? Or why did the agent refuse?</div>
            </div>
            <div className="text-center">
              <div className="font-mono text-[0.78rem] font-semibold text-accent mb-1">get_event</div>
              <div className="text-[0.78rem] text-fg-muted">Look up any evidence entry by ID.</div>
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
        <p className="text-fg-muted mb-10 text-[1.14rem]">Full OpenAPI 3.0 documentation for all endpoints, including webhook ingress and hosted analytics routes.</p>
        <a href="/docs/api" className="flex items-center justify-between bg-bg-elevated border border-border rounded-[10px] p-6 px-8 shadow-[var(--shadow-card)] transition-all hover:shadow-[var(--shadow-card-lg)] hover:border-accent no-underline">
          <div>
            <h3 className="text-base text-fg mb-1">Interactive API Documentation</h3>
            <p className="text-[0.85rem] text-fg-muted">Explore all 17 endpoints with request/response schemas, authentication details, examples, webhook payloads, and hosted scorecard/explain analytics contracts.</p>
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
