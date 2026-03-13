import { useState, useCallback, useRef, useEffect } from "react";
import { useNavigate } from "react-router";
import { useAuth } from "../context/AuthContext";
import { CodeBlock } from "../components/CodeBlock";

type Step = "invite" | "label" | "reveal" | "configure";
type EditorTab = "claude-code" | "json-config" | "codex" | "gemini";

const EDITOR_TABS: { id: EditorTab; label: string }[] = [
  { id: "claude-code", label: "Claude Code" },
  { id: "json-config", label: "Cursor / Claude Desktop" },
  { id: "codex", label: "Codex" },
  { id: "gemini", label: "Gemini CLI" },
];

type ConfigMode = "hosted" | "self-hosted" | "local";

const HOSTED_MCP_URL = "https://evidra.cc/mcp";

function mcpConfigWithKey(editor: EditorTab, apiKey: string, serverUrl: string, configMode: ConfigMode): string {
  if (configMode === "hosted") {
    const url = HOSTED_MCP_URL;
    const authHeader = `Bearer ${apiKey}`;

    if (editor === "claude-code") {
      return `claude mcp add --transport http \\
  -H "Authorization: ${authHeader}" \\
  -s user evidra ${url}`;
    }

    if (editor === "codex") {
      return `# ~/.codex/config.toml
[mcp_servers.evidra]
url = "${url}"

[mcp_servers.evidra.headers]
Authorization = "${authHeader}"`;
    }

    const jsonObj = {
      mcpServers: {
        evidra: {
          url,
          headers: { Authorization: authHeader },
        },
      },
    };

    if (editor === "gemini") {
      return `// ~/.gemini/settings.json\n${JSON.stringify(jsonObj, null, 2)}`;
    }
    return JSON.stringify(jsonObj, null, 2);
  }

  if (configMode === "self-hosted") {
    if (editor === "claude-code") {
      return `claude mcp add evidra -- evidra-mcp \\
  --evidence-dir ~/.evidra/evidence \\
  --environment production \\
  --signing-mode optional \\
  --url ${serverUrl} \\
  --api-key ${apiKey} \\
  --fallback-offline`;
    }

    const envBlock = {
      EVIDRA_EVIDENCE_DIR: "~/.evidra/evidence",
      EVIDRA_ENVIRONMENT: "production",
      EVIDRA_URL: serverUrl,
      EVIDRA_API_KEY: apiKey,
      EVIDRA_FALLBACK: "offline",
    };

    if (editor === "codex") {
      return `# ~/.codex/config.toml
[mcp_servers.evidra]
command = "evidra-mcp"
args = ["--signing-mode", "optional"]

[mcp_servers.evidra.env]
EVIDRA_EVIDENCE_DIR = "~/.evidra/evidence"
EVIDRA_ENVIRONMENT = "production"
EVIDRA_URL = "${serverUrl}"
EVIDRA_API_KEY = "${apiKey}"
EVIDRA_FALLBACK = "offline"`;
    }

    const jsonObj = {
      mcpServers: {
        evidra: {
          command: "evidra-mcp",
          args: ["--signing-mode", "optional"],
          env: envBlock,
        },
      },
    };

    if (editor === "gemini") {
      return `// ~/.gemini/settings.json\n${JSON.stringify(jsonObj, null, 2)}`;
    }
    return JSON.stringify(jsonObj, null, 2);
  }

  // Local mode — no URL/API key, just evidence dir + environment
  if (editor === "claude-code") {
    return `claude mcp add evidra -- evidra-mcp \\
  --evidence-dir ~/.evidra/evidence \\
  --environment development \\
  --signing-mode optional`;
  }

  const localEnv = {
    EVIDRA_EVIDENCE_DIR: "~/.evidra/evidence",
    EVIDRA_ENVIRONMENT: "development",
  };

  if (editor === "codex") {
    return `# ~/.codex/config.toml
[mcp_servers.evidra]
command = "evidra-mcp"
args = ["--signing-mode", "optional"]

[mcp_servers.evidra.env]
EVIDRA_EVIDENCE_DIR = "~/.evidra/evidence"
EVIDRA_ENVIRONMENT = "development"`;
  }

  const localObj = {
    mcpServers: {
      evidra: {
        command: "evidra-mcp",
        args: ["--signing-mode", "optional"],
        env: localEnv,
      },
    },
  };

  if (editor === "gemini") {
    return `// ~/.gemini/settings.json\n${JSON.stringify(localObj, null, 2)}`;
  }
  return JSON.stringify(localObj, null, 2);
}

interface KeyResponse {
  key: string;
  prefix: string;
  tenant_id: string;
  created_at: string;
}

export function Onboarding() {
  const [step, setStep] = useState<Step>("invite");
  const [inviteSecret, setInviteSecret] = useState("");
  const [label, setLabel] = useState("");
  const [apiKey, setApiKeyLocal] = useState<KeyResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);
  const [revealed, setRevealed] = useState(false);
  const [editor, setEditor] = useState<EditorTab>("claude-code");
  const [configMode, setConfigMode] = useState<ConfigMode>("hosted");
  const { setApiKey } = useAuth();
  const navigate = useNavigate();
  const keyRef = useRef<HTMLDivElement>(null);

  // Derive server URL from current location.
  const serverUrl = typeof window !== "undefined"
    ? `${window.location.protocol}//${window.location.host}`
    : "http://localhost:8080";

  const handleInviteContinue = useCallback(() => {
    if (!inviteSecret.trim()) {
      setError("Invite secret is required.");
      return;
    }
    setError(null);
    setStep("label");
  }, [inviteSecret]);

  const handleGenerateKey = useCallback(async () => {
    setError(null);
    setLoading(true);
    try {
      const res = await fetch("/v1/keys", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Invite-Secret": inviteSecret,
        },
        body: JSON.stringify({ label: label.trim() || undefined }),
      });

      if (!res.ok) {
        let msg = "Key generation failed.";
        try {
          const body = await res.json();
          msg = body.error || body.message || msg;
        } catch {
          // Non-JSON error body.
        }
        if (res.status === 403) msg = "Invalid invite secret.";
        if (res.status === 429) msg = "Rate limit exceeded. Try again later.";
        setError(msg);
        if (res.status === 403) setStep("invite");
        return;
      }

      const data: KeyResponse = await res.json();
      setApiKeyLocal(data);
      setApiKey(data.key);
      setStep("reveal");
    } catch {
      setError("Network error. Is the API server running?");
    } finally {
      setLoading(false);
    }
  }, [inviteSecret, label, setApiKey]);

  const handleCopy = useCallback(() => {
    if (!apiKey) return;
    navigator.clipboard.writeText(apiKey.key).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [apiKey]);

  // Trigger the reveal animation after mount.
  useEffect(() => {
    if (step === "reveal") {
      const timer = setTimeout(() => setRevealed(true), 100);
      return () => clearTimeout(timer);
    }
  }, [step]);

  const isAfter = (target: Step, current: Step) => {
    const order: Step[] = ["invite", "label", "reveal", "configure"];
    return order.indexOf(current) > order.indexOf(target);
  };

  const configPathNote =
    editor === "json-config"
      ? "Add to .cursor/mcp.json (Cursor), claude_desktop_config.json (Claude Desktop), or ~/.codeium/windsurf/mcp_config.json (Windsurf)."
      : editor === "codex"
        ? "Edit ~/.codex/config.toml."
        : editor === "gemini"
          ? "Edit ~/.gemini/settings.json."
          : "Run in your terminal.";

  return (
    <section className="relative min-h-[calc(100vh-60px)] py-16 bg-[radial-gradient(ellipse_60%_40%_at_50%_0%,var(--color-accent-subtle),var(--color-bg)_70%)]">
      {/* Dot grid texture */}
      <div className="absolute inset-0 bg-[radial-gradient(circle,var(--color-accent)_1px,transparent_1px)] bg-[length:24px_24px] opacity-[0.04] [mask-image:radial-gradient(ellipse_50%_50%_at_50%_20%,black,transparent)]" />

      <div className="relative max-w-[560px] mx-auto px-6">
        {/* Header */}
        <div className="text-center mb-12">
          <div className="inline-flex items-center gap-2 font-mono text-[0.72rem] font-medium text-accent tracking-widest uppercase mb-4">
            <span className="w-1.5 h-1.5 rounded-full bg-accent inline-block" />
            Onboarding
          </div>
          <h1 className="text-[1.6rem] font-extrabold text-fg tracking-tight mb-3">
            Get Your API Key
          </h1>
          <p className="text-[0.92rem] text-fg-muted leading-relaxed">
            Generate a key and configure your MCP server to connect to the Evidra API.
          </p>
        </div>

        {/* Progress indicator */}
        <div className="flex items-center justify-center gap-2 mb-10">
          <StepDot active={step === "invite"} complete={isAfter("invite", step)} label="1" />
          <StepLine active={isAfter("invite", step)} />
          <StepDot active={step === "label"} complete={isAfter("label", step)} label="2" />
          <StepLine active={isAfter("label", step)} />
          <StepDot active={step === "reveal"} complete={isAfter("reveal", step)} label="3" />
          <StepLine active={isAfter("reveal", step)} />
          <StepDot active={step === "configure"} complete={false} label="4" />
        </div>

        {/* Error toast */}
        {error && (
          <div className="mb-6 px-4 py-3 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800/50 rounded-lg text-[0.84rem] text-red-700 dark:text-red-300 font-medium animate-[shake_0.3s_ease-in-out]">
            {error}
          </div>
        )}

        {/* Step 1: Invite Secret */}
        <StepCard
          number="01"
          title="Invite Secret"
          subtitle="Provided by your team or instance admin"
          active={step === "invite"}
          complete={isAfter("invite", step)}
        >
          <div className="mt-4">
            <input
              type="password"
              value={inviteSecret}
              onChange={(e) => setInviteSecret(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleInviteContinue()}
              placeholder="paste your invite secret"
              autoFocus
              className="w-full px-4 py-3 bg-[var(--color-code-bg)] border border-border rounded-lg font-mono text-[0.88rem] text-fg placeholder:text-fg-muted/40 outline-none transition-all focus:border-accent focus:shadow-[0_0_0_3px_rgba(5,150,105,0.12)]"
            />
            <button
              onClick={handleInviteContinue}
              className="mt-4 w-full px-5 py-3 rounded-lg text-[0.88rem] font-semibold bg-accent text-white transition-all hover:bg-accent-bright hover:-translate-y-px hover:shadow-lg cursor-pointer border-none"
            >
              Continue
            </button>
          </div>
        </StepCard>

        {/* Step 2: Label */}
        <StepCard
          number="02"
          title="Label Your Key"
          subtitle="Optional — helps identify this key later"
          active={step === "label"}
          complete={isAfter("label", step)}
        >
          <div className="mt-4">
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleGenerateKey()}
              placeholder="e.g. production-ci, my-laptop"
              maxLength={128}
              autoFocus={step === "label"}
              className="w-full px-4 py-3 bg-[var(--color-code-bg)] border border-border rounded-lg font-mono text-[0.88rem] text-fg placeholder:text-fg-muted/40 outline-none transition-all focus:border-accent focus:shadow-[0_0_0_3px_rgba(5,150,105,0.12)]"
            />
            <div className="flex gap-3 mt-4">
              <button
                onClick={() => { setStep("invite"); setError(null); }}
                className="px-4 py-3 rounded-lg text-[0.84rem] font-medium bg-transparent border border-border text-fg-muted transition-all hover:border-accent hover:text-fg cursor-pointer"
              >
                Back
              </button>
              <button
                onClick={handleGenerateKey}
                disabled={loading}
                className="flex-1 px-5 py-3 rounded-lg text-[0.88rem] font-semibold bg-accent text-white transition-all hover:bg-accent-bright hover:-translate-y-px hover:shadow-lg cursor-pointer border-none disabled:opacity-60 disabled:cursor-not-allowed disabled:hover:translate-y-0"
              >
                {loading ? (
                  <span className="inline-flex items-center gap-2">
                    <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    Generating...
                  </span>
                ) : (
                  "Generate Key"
                )}
              </button>
            </div>
          </div>
        </StepCard>

        {/* Step 3: Key Reveal */}
        <StepCard
          number="03"
          title="Your API Key"
          subtitle="This is shown once — copy it now"
          active={step === "reveal"}
          complete={isAfter("reveal", step)}
        >
          {apiKey && (
            <div className="mt-4">
              {/* Key display with reveal animation */}
              <div
                ref={keyRef}
                className="relative bg-[var(--color-code-bg)] border border-accent/30 rounded-lg p-4 overflow-hidden"
              >
                <div className="flex items-center justify-between gap-3">
                  <code
                    className={`font-mono text-[0.92rem] font-medium text-fg tracking-wide break-all transition-all duration-700 ${
                      revealed ? "opacity-100 blur-0" : "opacity-0 blur-sm"
                    }`}
                  >
                    {apiKey.key}
                  </code>
                  <button
                    onClick={handleCopy}
                    className="shrink-0 px-3 py-1.5 rounded-md text-[0.78rem] font-mono font-medium bg-accent/10 border border-accent/20 text-accent transition-all hover:bg-accent/20 cursor-pointer"
                  >
                    {copied ? "copied!" : "copy"}
                  </button>
                </div>
                {/* Seal line animation */}
                <div
                  className={`absolute bottom-0 left-0 h-[2px] bg-accent transition-all duration-1000 ease-out ${
                    revealed ? "w-full" : "w-0"
                  }`}
                />
              </div>

              {/* Key metadata */}
              <div className="mt-4 grid grid-cols-2 gap-3">
                <MetaField label="Prefix" value={apiKey.prefix} />
                <MetaField label="Tenant" value={apiKey.tenant_id} />
              </div>

              {/* Warning */}
              <div className="mt-5 px-4 py-3 bg-amber-50 dark:bg-amber-950/20 border border-amber-200 dark:border-amber-800/40 rounded-lg text-[0.82rem] text-amber-800 dark:text-amber-300 leading-relaxed">
                Store this key securely. It cannot be retrieved after you leave this page.
              </div>

              <button
                onClick={() => setStep("configure")}
                className="mt-5 w-full px-5 py-3 rounded-lg text-[0.88rem] font-semibold bg-accent text-white transition-all hover:bg-accent-bright hover:-translate-y-px hover:shadow-lg cursor-pointer border-none"
              >
                Configure MCP Server
              </button>
            </div>
          )}
        </StepCard>

        {/* Step 4: MCP Configuration */}
        <StepCard
          number="04"
          title="Configure Your MCP Server"
          subtitle="Copy the config with your key pre-filled"
          active={step === "configure"}
          complete={false}
        >
          {apiKey && (
            <div className="mt-4">
              {/* Mode toggle: Hosted / Self-hosted / Local */}
              <div className="flex items-center gap-3 mb-4">
                <span className="text-[0.76rem] font-medium text-fg-muted">Mode:</span>
                {(["hosted", "self-hosted", "local"] as ConfigMode[]).map((m) => (
                  <button
                    key={m}
                    onClick={() => setConfigMode(m)}
                    className={`px-3 py-1.5 rounded-lg text-[0.78rem] font-mono font-medium border transition-all cursor-pointer ${
                      configMode === m
                        ? "bg-accent/10 border-accent/30 text-accent"
                        : "bg-transparent border-border-subtle text-fg-muted hover:border-accent/20 hover:text-fg"
                    }`}
                  >
                    {m === "hosted" ? "Hosted" : m === "self-hosted" ? "Self-hosted" : "Local only"}
                  </button>
                ))}
              </div>

              {configMode === "hosted" && (
                <div className="mb-4 px-4 py-3 bg-accent-subtle border border-accent/20 rounded-lg text-[0.82rem] text-fg-muted leading-relaxed">
                  Connect directly to <code className="text-[0.78rem] text-fg">{HOSTED_MCP_URL}</code> via HTTP transport. No local binary required — your editor connects to the hosted Evidra MCP server.
                </div>
              )}
              {configMode === "self-hosted" && (
                <div className="mb-4 px-4 py-3 bg-accent-subtle border border-accent/20 rounded-lg text-[0.82rem] text-fg-muted leading-relaxed">
                  Evidence is stored locally <em>and</em> forwarded to <code className="text-[0.78rem] text-fg">{serverUrl}</code>. Requires <code className="text-[0.78rem] text-fg">evidra-mcp</code> binary installed locally. Falls back to local-only if the API is unreachable.
                </div>
              )}
              {configMode === "local" && (
                <div className="mb-4 px-4 py-3 bg-[var(--color-code-bg)] border border-border rounded-lg text-[0.82rem] text-fg-muted leading-relaxed">
                  Evidence is stored locally in <code className="text-[0.78rem] text-fg">~/.evidra/evidence</code>. Requires <code className="text-[0.78rem] text-fg">evidra-mcp</code> binary installed locally. No API connection required. Use <code className="text-[0.78rem] text-fg">evidra scorecard</code> to analyze locally.
                </div>
              )}

              {/* Editor tabs */}
              <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px] mb-4 flex-wrap">
                {EDITOR_TABS.map((tab) => (
                  <button
                    key={tab.id}
                    onClick={() => setEditor(tab.id)}
                    className={`border-none rounded-md px-3 py-1.5 cursor-pointer text-[0.78rem] font-semibold font-sans transition-all ${
                      editor === tab.id
                        ? "bg-bg-elevated text-fg shadow-[var(--shadow-card)]"
                        : "bg-transparent text-fg-muted hover:text-fg"
                    }`}
                  >
                    {tab.label}
                  </button>
                ))}
              </div>

              <p className="text-[0.82rem] text-fg-muted mb-3">{configPathNote}</p>

              {/* Config with real key injected */}
              <CodeBlock code={mcpConfigWithKey(editor, apiKey.key, serverUrl, configMode)} />

              {/* Args & env reference */}
              <details className="mt-5 group">
                <summary className="cursor-pointer font-mono text-[0.74rem] font-medium tracking-wider uppercase text-fg-muted/60 hover:text-accent transition-colors select-none list-none flex items-center gap-2">
                  <span className="text-[0.7rem] transition-transform group-open:rotate-90">&#9654;</span>
                  All flags &amp; environment variables
                </summary>
                <div className="mt-3 bg-[var(--color-code-bg)] border border-border rounded-lg overflow-hidden">
                  <table className="w-full text-left text-[0.78rem]">
                    <thead>
                      <tr className="border-b border-border">
                        <th className="px-4 py-2 font-mono font-medium text-fg-muted/60 text-[0.72rem] uppercase tracking-wider">Flag</th>
                        <th className="px-4 py-2 font-mono font-medium text-fg-muted/60 text-[0.72rem] uppercase tracking-wider">Env Variable</th>
                        <th className="px-4 py-2 font-mono font-medium text-fg-muted/60 text-[0.72rem] uppercase tracking-wider">Default</th>
                        <th className="px-4 py-2 font-medium text-fg-muted/60 text-[0.72rem] uppercase tracking-wider">Description</th>
                      </tr>
                    </thead>
                    <tbody className="font-mono text-[0.76rem]">
                      <tr className="border-b border-border-subtle">
                        <td className="px-4 py-2 text-fg"><code>--evidence-dir</code></td>
                        <td className="px-4 py-2 text-fg-muted"><code>EVIDRA_EVIDENCE_DIR</code></td>
                        <td className="px-4 py-2 text-fg-muted/60">~/.evidra/evidence</td>
                        <td className="px-4 py-2 text-fg-muted font-sans">Local evidence storage path</td>
                      </tr>
                      <tr className="border-b border-border-subtle">
                        <td className="px-4 py-2 text-fg"><code>--environment</code></td>
                        <td className="px-4 py-2 text-fg-muted"><code>EVIDRA_ENVIRONMENT</code></td>
                        <td className="px-4 py-2 text-fg-muted/60">(none)</td>
                        <td className="px-4 py-2 text-fg-muted font-sans">Label: production, staging, development</td>
                      </tr>
                      <tr className="border-b border-border-subtle">
                        <td className="px-4 py-2 text-fg"><code>--signing-mode</code></td>
                        <td className="px-4 py-2 text-fg-muted"><code>EVIDRA_SIGNING_MODE</code></td>
                        <td className="px-4 py-2 text-fg-muted/60">strict</td>
                        <td className="px-4 py-2 text-fg-muted font-sans">strict or optional (use optional for dev)</td>
                      </tr>
                      <tr className="border-b border-border-subtle">
                        <td className="px-4 py-2 text-fg"><code>--url</code></td>
                        <td className="px-4 py-2 text-fg-muted"><code>EVIDRA_URL</code></td>
                        <td className="px-4 py-2 text-fg-muted/60">(none)</td>
                        <td className="px-4 py-2 text-fg-muted font-sans">API server URL for online forwarding</td>
                      </tr>
                      <tr className="border-b border-border-subtle">
                        <td className="px-4 py-2 text-fg"><code>--api-key</code></td>
                        <td className="px-4 py-2 text-fg-muted"><code>EVIDRA_API_KEY</code></td>
                        <td className="px-4 py-2 text-fg-muted/60">(none)</td>
                        <td className="px-4 py-2 text-fg-muted font-sans">Bearer token for API auth</td>
                      </tr>
                      <tr className="border-b border-border-subtle">
                        <td className="px-4 py-2 text-fg"><code>--fallback-offline</code></td>
                        <td className="px-4 py-2 text-fg-muted"><code>EVIDRA_FALLBACK</code></td>
                        <td className="px-4 py-2 text-fg-muted/60">(none)</td>
                        <td className="px-4 py-2 text-fg-muted font-sans">Set to &ldquo;offline&rdquo; to fall back when API is unreachable</td>
                      </tr>
                      <tr className="border-b border-border-subtle">
                        <td className="px-4 py-2 text-fg"><code>--retry-tracker</code></td>
                        <td className="px-4 py-2 text-fg-muted"><code>EVIDRA_RETRY_TRACKER</code></td>
                        <td className="px-4 py-2 text-fg-muted/60">false</td>
                        <td className="px-4 py-2 text-fg-muted font-sans">Enable retry loop signal detection</td>
                      </tr>
                      <tr>
                        <td className="px-4 py-2 text-fg">&mdash;</td>
                        <td className="px-4 py-2 text-fg-muted"><code>EVIDRA_SIGNING_KEY</code></td>
                        <td className="px-4 py-2 text-fg-muted/60">(none)</td>
                        <td className="px-4 py-2 text-fg-muted font-sans">Base64 Ed25519 private key (or use EVIDRA_SIGNING_KEY_PATH)</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </details>

              {/* Verify instructions */}
              <div className="mt-5 bg-[var(--color-code-bg)] border border-border rounded-lg px-5 py-4">
                <div className="font-mono text-[0.72rem] font-medium tracking-widest uppercase text-accent mb-2">
                  Verify
                </div>
                <p className="text-[0.82rem] text-fg-muted leading-relaxed">
                  Restart your editor, then ask your agent: <em>&ldquo;What tools do you have from Evidra?&rdquo;</em>
                  &mdash; you should see <code className="text-[0.78rem] text-fg">prescribe</code>, <code className="text-[0.78rem] text-fg">report</code>, and <code className="text-[0.78rem] text-fg">get_event</code>.
                </p>
              </div>

              {/* Final navigation */}
              <div className="mt-8 pt-6 border-t border-border-subtle">
                <div className="font-mono text-[0.72rem] font-medium tracking-widest uppercase text-accent mb-4">
                  You&apos;re All Set
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <button
                    onClick={() => navigate("/dashboard")}
                    className="px-4 py-3 rounded-lg text-[0.84rem] font-semibold bg-accent text-white transition-all hover:bg-accent-bright hover:-translate-y-px cursor-pointer border-none text-center"
                  >
                    Open Dashboard
                  </button>
                  <a
                    href="https://github.com/vitas/evidra/blob/main/docs/guides/mcp-setup.md"
                    target="_blank"
                    rel="noopener"
                    className="px-4 py-3 rounded-lg text-[0.84rem] font-semibold bg-transparent border border-border text-fg-muted transition-all hover:border-accent hover:text-fg no-underline text-center"
                  >
                    Full MCP Guide
                  </a>
                </div>
              </div>
            </div>
          )}
        </StepCard>
      </div>
    </section>
  );
}

/* ── Sub-components ── */

function StepCard({
  number,
  title,
  subtitle,
  active,
  complete,
  children,
}: {
  number: string;
  title: string;
  subtitle: string;
  active: boolean;
  complete: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div
      className={`mb-4 rounded-xl border transition-all duration-300 overflow-hidden ${
        active
          ? "bg-bg-elevated border-accent/30 shadow-[var(--shadow-card-lg)] border-l-[3px] border-l-accent"
          : complete
            ? "bg-bg-elevated/60 border-border opacity-60"
            : "bg-bg-alt/40 border-border-subtle opacity-40"
      }`}
    >
      <div className="px-6 py-5">
        <div className="flex items-center gap-3 mb-1">
          <span
            className={`font-mono text-[0.72rem] font-medium tracking-wider ${
              active ? "text-accent" : complete ? "text-accent/60" : "text-fg-muted/40"
            }`}
          >
            {complete ? "\u2713" : number}
          </span>
          <h3
            className={`text-[0.95rem] font-bold tracking-tight ${
              active ? "text-fg" : "text-fg-muted"
            }`}
          >
            {title}
          </h3>
        </div>
        <p
          className={`text-[0.82rem] ml-[1.6rem] ${
            active ? "text-fg-muted" : "text-fg-muted/50"
          }`}
        >
          {subtitle}
        </p>
        {active && children}
      </div>
    </div>
  );
}

function StepDot({
  active,
  complete,
  label,
}: {
  active: boolean;
  complete: boolean;
  label: string;
}) {
  return (
    <div
      className={`w-7 h-7 rounded-full flex items-center justify-center text-[0.7rem] font-mono font-semibold transition-all duration-300 ${
        active
          ? "bg-accent text-white shadow-[0_0_8px_rgba(5,150,105,0.4)]"
          : complete
            ? "bg-accent/20 text-accent border border-accent/30"
            : "bg-bg-alt text-fg-muted/40 border border-border-subtle"
      }`}
    >
      {complete ? "\u2713" : label}
    </div>
  );
}

function StepLine({ active }: { active: boolean }) {
  return (
    <div
      className={`w-8 h-px transition-colors duration-500 ${
        active ? "bg-accent/40" : "bg-border-subtle"
      }`}
    />
  );
}

function MetaField({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-[var(--color-code-bg)] rounded-lg px-3 py-2.5 border border-border-subtle">
      <div className="font-mono text-[0.68rem] font-medium text-fg-muted/60 uppercase tracking-wider mb-0.5">
        {label}
      </div>
      <div className="font-mono text-[0.82rem] text-fg truncate">{value}</div>
    </div>
  );
}
