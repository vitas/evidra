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

const FEATURES = [
  { icon: "\u25CE", title: "Observe", desc: "Record every infrastructure operation as signed evidence in an append-only, hash-linked chain." },
  { icon: "\u25A4", title: "Measure", desc: "Detect 8 behavioral signals: retry loops, artifact drift, protocol violations, blast radius, and more." },
  { icon: "\u2605", title: "Score", desc: "Compute reliability scorecards with weighted penalties, safety floors, and confidence levels." },
  { icon: "\u21C4", title: "Compare", desc: "Compare actors, tools, and scopes side-by-side with workload overlap analysis." },
];

const GUIDES = [
  { tag: "CI / CD", title: "CI Integration", desc: "Add reliability scoring to your CI pipeline \u2014 Terraform, Kubernetes, Docker, and more.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/terraform-ci-quickstart.md" },
  { tag: "Observability", title: "Metrics Export", desc: "Export signals and scores to Grafana, Datadog, or any OTLP-compatible backend.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/observability-quickstart.md" },
  { tag: "Scanners", title: "SARIF Integration", desc: "Ingest findings from Trivy, Kubescape, or any SARIF-compatible security scanner.", href: "https://github.com/vitas/evidra/blob/main/docs/integrations/SCANNER_SARIF_QUICKSTART.md" },
];

export function Landing() {
  return (
    <>
      <Hero />
      <Divider />
      <Features />
      <Divider />
      <Architecture />
      <Divider />
      <GettingStarted />
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
        <div className="grid grid-cols-3 gap-5 max-md:grid-cols-1">
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
