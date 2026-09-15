import { CodeBlock } from "../components/CodeBlock";

function Container({ children }: { children: React.ReactNode }) {
  return <div className="mx-auto w-full max-w-[72rem] px-4">{children}</div>;
}

function Eyebrow({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[0.8rem] font-semibold uppercase tracking-[0.08em] text-accent mb-2">
      {children}
    </p>
  );
}

export function Landing() {
  return (
    <>
      <Hero />
      <RuntimeTopology />
      <SourceBoundaries />
      <ReconciliationExample />
      <TrustBoundaries />
      <Workflow />
      <UseCases />
      <OpenSource />
      <Faq />
      <FinalCta />
    </>
  );
}

function Hero() {
  return (
    <section id="hero" className="py-16 sm:py-20">
      <Container>
        <div className="hero-copy">
          <Eyebrow>Open-source MCP execution evidence</Eyebrow>
          <h1 className="text-[2.4rem] sm:text-[3rem] leading-tight font-bold text-fg tracking-tight">
            Verifiable evidence from the MCP execution boundary.
          </h1>
          <p className="mt-4 text-[1.15rem] text-fg-muted">
            Evidra records what an agent declared, what the proxy observed, and
            what the agent reported&mdash;without confusing any one of them with
            external truth.
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <a
              className="btn-primary"
              href="https://github.com/vitas/evidra/blob/main/docs/getting-started.md"
              target="_blank"
              rel="noopener"
            >
              Get started
            </a>
            <a
              className="btn-secondary"
              href="https://github.com/vitas/evidra"
              target="_blank"
              rel="noopener"
            >
              View on GitHub
            </a>
          </div>
          <p className="mt-4 text-[0.85rem] text-fg-muted">
            <span className="block">
              The relaunched Core line is currently an unreleased preview built
              from main.
            </span>
            <span className="block">
              Build from source; no hosted service required.
            </span>
          </p>
        </div>
      </Container>
    </section>
  );
}

function RuntimeTopology() {
  return (
    <section id="runtime-topology" className="py-12 bg-bg-alt">
      <Container>
        <Eyebrow>How it runs</Eyebrow>
        <h2 className="section-title">Runtime topology</h2>
        <p className="mt-3 text-fg-muted max-w-[60ch]">
          One endpoint wraps one upstream MCP server; the agent sees a single
          server in its tool list.
        </p>
        <div
          className="topology mt-8"
          role="img"
          aria-label="Agent connects to the Evidra MCP endpoint, which forwards calls to one upstream MCP server and writes a signed evidence directory."
        >
          <div className="topology-node">Agent</div>
          <div className="topology-arrow" aria-hidden="true">
            &darr; &uarr;
          </div>
          <div className="topology-node topology-node-primary">
            Evidra MCP endpoint
            <span className="topology-note">
              merges evidra_prescribe / evidra_report into the tool list
            </span>
          </div>
          <div className="topology-branches">
            <div className="topology-branch">
              <span className="topology-arrow" aria-hidden="true">
                &rarr;
              </span>
              <div className="topology-node">Upstream MCP server</div>
            </div>
            <div className="topology-branch">
              <span className="topology-arrow" aria-hidden="true">
                &darr;
              </span>
              <div className="topology-node">Signed evidence directory</div>
            </div>
          </div>
        </div>
        <p className="mt-6 text-[0.9rem] text-fg-muted max-w-[60ch]">
          Tool calls the agent makes are forwarded to the upstream server
          unchanged. The endpoint appends each step to a local, Ed25519-signed
          hash chain as it happens, and nothing about the upstream&rsquo;s work
          is simulated or inferred on the way.
        </p>
      </Container>
    </section>
  );
}

const BOUNDARIES = [
  {
    title: "Declared",
    desc: "The objective and expected outcome supplied by the agent.",
  },
  {
    title: "Observed",
    desc: "Execution boundaries, status, timing, metadata, and bounded keyed fingerprints recorded by the proxy.",
  },
  {
    title: "Reported",
    desc: "The final status and outcome claimed by the agent.",
  },
];

function SourceBoundaries() {
  return (
    <section id="source-boundaries" className="py-12">
      <Container>
        <Eyebrow>Three sources, never merged</Eyebrow>
        <h2 className="section-title">Declared, observed, reported</h2>
        <p className="mt-3 text-fg-muted max-w-[60ch]">
          Every record keeps its origin. A claim never becomes an observation,
          and an observation never becomes a verdict. Reconciliation is what a
          human reads.
        </p>
        <div className="card-grid mt-8">
          {BOUNDARIES.map((b) => (
            <div key={b.title} className="card">
              <h3 className="font-mono text-[0.95rem] font-semibold text-accent">
                {b.title}
              </h3>
              <p className="mt-2 text-[0.9rem] text-fg-muted">{b.desc}</p>
            </div>
          ))}
        </div>
      </Container>
    </section>
  );
}

function ReconciliationExample() {
  return (
    <section id="reconciliation-example" className="py-12 bg-bg-alt">
      <Container>
        <Eyebrow>Example</Eyebrow>
        <h2 className="section-title">What reconciliation reads like</h2>
        <div className="card mt-8 p-6">
          <dl className="reconciliation">
            <div className="reconciliation-row">
              <dt>Declared</dt>
              <dd>Restart the deployment and confirm recovery</dd>
            </div>
            <div className="reconciliation-row">
              <dt>Observed</dt>
              <dd>
                restart_deployment &rarr; success; get_status &rarr; success;
                result fingerprint present
              </dd>
            </div>
            <div className="reconciliation-row">
              <dt>Reported</dt>
              <dd>completed / achieved</dd>
            </div>
            <div className="reconciliation-row">
              <dt>Reviewer interpretation</dt>
              <dd>
                The proxy observed the requested calls and successful
                responses. External application health was not independently
                verified.
              </dd>
            </div>
          </dl>
        </div>
        <p className="mt-4 text-[0.85rem] text-fg-muted">
          Evidra generates the reconciliation summary from the recorded chain;
          a human judges what it means for the external outcome.
        </p>
      </Container>
    </section>
  );
}

const TRUST_BOUNDARIES = [
  "Evidra is not a sandbox: the upstream server executes with its existing authority. When recording is healthy, Evidra records the MCP boundary. Known degraded windows are reported, and unknown loss can still exist.",
  "A successful tool response is not proof of the external outcome. A human judges the external outcome against external state.",
  "Declared and reported entries are agent claims. Their origin and position are preserved only within the local recorder trust boundary; they do not establish organizational identity.",
  "Raw observed arguments and response bodies are not stored. Arguments use keyed HMAC digests; results use a bounded fingerprint or an explicit omitted or unavailable status.",
  "Chain validity, signature validity, and evidence coverage are separate conclusions. A valid chain can still be incomplete.",
];

function TrustBoundaries() {
  return (
    <section id="trust-boundaries" className="py-12">
      <Container>
        <Eyebrow>Trust boundaries</Eyebrow>
        <h2 className="section-title">What the evidence does not claim</h2>
        <ul className="mt-8 space-y-3 max-w-[70ch]">
          {TRUST_BOUNDARIES.map((line) => (
            <li key={line} className="trust-item">
              {line}
            </li>
          ))}
        </ul>
      </Container>
    </section>
  );
}

function Workflow() {
  return (
    <section id="workflow" className="py-12 bg-bg-alt">
      <Container>
        <Eyebrow>Workflow</Eyebrow>
        <h2 className="section-title">One operation, then verify and summarize</h2>
        <div className="grid gap-8 mt-8 lg:grid-cols-2">
          <div>
            <h3 className="step-title">1 &middot; Wrap your MCP server</h3>
            <p className="step-desc">
              The endpoint starts the upstream as a child process and merges two
              protocol tools into its list. Protocol order is enforced: while no
              operation is open, tool calls are refused and noted.
            </p>
            <CodeBlock code="evidra-mcp --proxy -- <your upstream MCP server>" />
          </div>
          <div>
            <h3 className="step-title">2 &middot; Reconcile afterwards</h3>
            <p className="step-desc">
              The read side verifies the chain and builds a summary that puts
              declarations, observations and reports side by side.
            </p>
            <CodeBlock
              code={`evidra summarize --dir ./evidence\nevidra verify --dir ./evidence`}
            />
          </div>
        </div>
      </Container>
    </section>
  );
}

const USE_CASES = [
  {
    title: "Incident reconstruction",
    desc: "After the fact, replay who declared what, what actually crossed the proxy, and in what order — from the signed chain, not from chat logs.",
  },
  {
    title: "Agent evaluation",
    desc: "Compare runs on evidence: voluntary protocol compliance, unprescribed mutations, retries and drift — each measured and reported per mode.",
  },
  {
    title: "Operational review",
    desc: "Give a reviewer a summary that separates claim from observation, so disagreement becomes a finding instead of a silent gap.",
  },
];

function UseCases() {
  return (
    <section id="use-cases" className="py-12">
      <Container>
        <Eyebrow>Use cases</Eyebrow>
        <h2 className="section-title">What people reconcile with it</h2>
        <div className="card-grid mt-8">
          {USE_CASES.map((u) => (
            <div key={u.title} className="card">
              <h3 className="font-semibold text-fg">{u.title}</h3>
              <p className="mt-2 text-[0.9rem] text-fg-muted">{u.desc}</p>
            </div>
          ))}
        </div>
      </Container>
    </section>
  );
}

function OpenSource() {
  return (
    <section id="open-source" className="py-12 bg-bg-alt">
      <Container>
        <Eyebrow>Open source</Eyebrow>
        <h2 className="section-title">Read every byte it writes</h2>
        <div className="card-grid mt-8">
          <div className="card">
            <h3 className="font-semibold text-fg">Apache-2.0</h3>
            <p className="mt-2 text-[0.9rem] text-fg-muted">
              The recorder, the read side, and the conformance fixture are one
              Go module with no hidden components.
            </p>
          </div>
          <div className="card">
            <h3 className="font-semibold text-fg">Inspectable JSONL</h3>
            <p className="mt-2 text-[0.9rem] text-fg-muted">
              The chain is a local file format with a published digest and
              signature specification; verify it with your own tools.
            </p>
          </div>
          <div className="card">
            <h3 className="font-semibold text-fg">Guarded claims</h3>
            <p className="mt-2 text-[0.9rem] text-fg-muted">
              Public statements are tied to executable guards in-repo; what has
              not been measured is written down as not measured.
            </p>
          </div>
        </div>
      </Container>
    </section>
  );
}

const FAQ_ITEMS = [
  {
    q: "Does Evidra verify that the external system actually changed?",
    a: "No. It records declarations, proxy observations, and reports, and keeps them attributable. External-state verification is outside the product boundary and is treated as a read-side reconciliation step.",
  },
  {
    q: "What does an agent have to do to be recorded?",
    a: "By default, --enforce=all requires an open prescribed operation and blocks an unprescribed call instead of forwarding it or creating an execution observation. When recording is enabled, Evidra attempts to append a protocol_violation; if that append fails, the endpoint follows its healthy-boundary failure behavior rather than claiming the record exists. With --enforce=off, observe-only mode forwards and records the call with an empty operation ID.",
  },
  {
    q: "Where do the keys live?",
    a: "In the local evidence directory: an Ed25519 signing key and an HMAC digest key, created per recording directory. They prove chain consistency within that directory's threat model; they do not prove organizational identity.",
  },
  {
    q: "Is there a hosted version?",
    a: "No. Evidra Core runs entirely on your machine against your own evidence directories. There is no account, no upload, and no service to depend on.",
  },
];

function Faq() {
  return (
    <section id="faq" className="py-12">
      <Container>
        <Eyebrow>FAQ</Eyebrow>
        <h2 className="section-title">Honest answers to the first questions</h2>
        <div className="mt-8 space-y-3 max-w-[70ch]">
          {FAQ_ITEMS.map((item) => (
            <details key={item.q} className="faq-item">
              <summary className="font-medium text-fg cursor-pointer">
                {item.q}
              </summary>
              <p className="mt-2 text-[0.9rem] text-fg-muted">{item.a}</p>
            </details>
          ))}
        </div>
      </Container>
    </section>
  );
}

function FinalCta() {
  return (
    <section id="final-cta" className="py-16 text-center">
      <Container>
        <h2 className="section-title">Record your first signed chain</h2>
        <p className="mt-3 text-fg-muted max-w-[60ch] mx-auto">
          Wrap one MCP server, run one real task, and read the summary while it
          is still fresh.
        </p>
        <div className="mt-8 flex flex-wrap gap-3 justify-center">
          <a
            className="btn-primary"
            href="https://github.com/vitas/evidra/blob/main/docs/getting-started.md"
            target="_blank"
            rel="noopener"
          >
            Run the two-minute setup
          </a>
          <a
            className="btn-secondary"
            href="https://github.com/vitas/evidra/blob/main/docs/evidence-format.md"
            target="_blank"
            rel="noopener"
          >
            Read the evidence format
          </a>
        </div>
      </Container>
    </section>
  );
}
