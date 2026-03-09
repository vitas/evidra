# React Landing Page Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the plain HTML landing page with a React 19 + Vite 6 + Tailwind v4 SPA, using the same go:embed pattern as evidra-mcp. Structure for future SaaS pages.

**Architecture:** Vite project in `ui/` builds to `ui/dist/`. Go embeds `ui/dist` via build tag `embed_ui` (same pattern as evidra-mcp). Router uses SPA fallback handler. Swagger UI and openapi.yaml stay as plain static files in `cmd/evidra-api/static/`.

**Tech Stack:** React 19, TypeScript 5, Vite 6, Tailwind CSS v4, Mermaid (npm), Vitest, Playwright.

---

### Task 1: Scaffold Vite + React + TypeScript project

**Files:**
- Create: `ui/package.json`
- Create: `ui/tsconfig.json`
- Create: `ui/vite.config.ts`
- Create: `ui/index.html`
- Create: `ui/src/main.tsx`
- Create: `ui/src/App.tsx`
- Create: `ui/src/vite-env.d.ts`

**Step 1: Create the project**

```bash
mkdir -p ui && cd ui
npm init -y
npm install react@19 react-dom@19
npm install -D typescript@latest @types/react@latest @types/react-dom@latest \
  vite@latest @vitejs/plugin-react@latest
```

**Step 2: Write `ui/vite.config.ts`**

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/v1": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
      "/healthz": { target: "http://localhost:8080", changeOrigin: true },
      "/readyz": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./test/setup.ts"],
    exclude: ["e2e/**", "node_modules/**"],
  },
});
```

**Step 3: Write `ui/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "forceConsistentCasingInFileNames": true
  },
  "include": ["src"]
}
```

**Step 4: Write `ui/index.html`**

```html
<!doctype html>
<html lang="en" data-theme="light">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Evidra — Reliability Scoring for Infrastructure Automation</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

**Step 5: Write `ui/src/vite-env.d.ts`**

```ts
/// <reference types="vite/client" />
```

**Step 6: Write `ui/src/main.tsx`**

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./App";
import "./styles/global.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
```

**Step 7: Write `ui/src/App.tsx`** (minimal placeholder)

```tsx
export function App() {
  return <div>Evidra</div>;
}
```

**Step 8: Verify it builds**

```bash
cd ui && npx vite build
```
Expected: `ui/dist/` created with `index.html` and `assets/`.

**Step 9: Commit**

```bash
git add ui/
git commit -m "feat(ui): scaffold Vite + React 19 + TypeScript project"
```

---

### Task 2: Add Tailwind v4 with emerald theme

**Files:**
- Modify: `ui/package.json` (add tailwindcss)
- Create: `ui/src/styles/global.css`

**Step 1: Install Tailwind v4**

```bash
cd ui
npm install -D tailwindcss @tailwindcss/vite
```

**Step 2: Add Tailwind plugin to `ui/vite.config.ts`**

Add import and plugin:

```ts
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // ... rest unchanged
});
```

**Step 3: Write `ui/src/styles/global.css`**

```css
@import "tailwindcss";

/* ── Theme tokens ── */
@theme {
  --color-bg: #fafffe;
  --color-bg-alt: #f0fdf4;
  --color-bg-elevated: #ffffff;
  --color-fg: #064e3b;
  --color-fg-body: #1a3a2a;
  --color-fg-muted: #6b8f7b;
  --color-accent: #059669;
  --color-accent-bright: #10b981;
  --color-accent-tint: #d1fae5;
  --color-accent-subtle: #ecfdf5;
  --color-border: #c6e8d6;
  --color-border-subtle: #e0f2e9;
  --font-sans: "Plus Jakarta Sans", -apple-system, sans-serif;
  --font-mono: "JetBrains Mono", monospace;
}

/* ── Dark theme overrides ── */
[data-theme="dark"] {
  --color-bg: #0c0f0e;
  --color-bg-alt: #111916;
  --color-bg-elevated: #161e1a;
  --color-fg: #d1fae5;
  --color-fg-body: #a7cdb8;
  --color-fg-muted: #6b8f7b;
  --color-accent: #34d399;
  --color-accent-bright: #6ee7b7;
  --color-accent-tint: #064e3b;
  --color-accent-subtle: #0d2818;
  --color-border: #1e3a2c;
  --color-border-subtle: #172b22;
}

/* ── Reset ── */
*, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: var(--font-sans);
  background: var(--color-bg);
  color: var(--color-fg-body);
  line-height: 1.65;
  -webkit-font-smoothing: antialiased;
  overflow-x: hidden;
}
a { color: var(--color-accent); text-decoration: none; transition: color 0.15s; }
a:hover { color: var(--color-accent-bright); }
```

**Step 4: Verify Tailwind works**

Update `App.tsx` temporarily:

```tsx
export function App() {
  return <div className="bg-accent text-white p-4">Tailwind works</div>;
}
```

```bash
cd ui && npx vite build
```
Expected: builds successfully, CSS includes Tailwind utilities.

**Step 5: Commit**

```bash
git add ui/
git commit -m "feat(ui): add Tailwind v4 with emerald theme tokens"
```

---

### Task 3: Add useTheme hook and ThemeToggle component

**Files:**
- Create: `ui/src/hooks/useTheme.ts`
- Create: `ui/src/components/ThemeToggle.tsx`
- Create: `ui/test/setup.ts`
- Create: `ui/test/components/ThemeToggle.test.tsx`
- Modify: `ui/package.json` (add vitest + testing-library)

**Step 1: Install test dependencies**

```bash
cd ui
npm install -D vitest jsdom @testing-library/react @testing-library/jest-dom @testing-library/user-event
```

**Step 2: Write `ui/test/setup.ts`**

```ts
import "@testing-library/jest-dom/vitest";

const createStorage = (): Storage => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = String(value); },
    removeItem: (key: string) => { delete store[key]; },
    clear: () => { store = {}; },
    get length() { return Object.keys(store).length; },
    key: (index: number) => Object.keys(store)[index] ?? null,
  };
};

Object.defineProperty(globalThis, "localStorage", { value: createStorage() });
Object.defineProperty(globalThis, "sessionStorage", { value: createStorage() });
```

**Step 3: Write the failing test**

`ui/test/components/ThemeToggle.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach } from "vitest";
import { ThemeToggle } from "../../src/components/ThemeToggle";

describe("ThemeToggle", () => {
  beforeEach(() => {
    document.documentElement.setAttribute("data-theme", "light");
    localStorage.clear();
  });

  it("renders a button", () => {
    render(<ThemeToggle />);
    expect(screen.getByRole("button", { name: /toggle theme/i })).toBeInTheDocument();
  });

  it("toggles theme on click", async () => {
    render(<ThemeToggle />);
    const btn = screen.getByRole("button", { name: /toggle theme/i });
    await userEvent.click(btn);
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});
```

**Step 4: Run test to verify it fails**

```bash
cd ui && npx vitest run --reporter=verbose
```
Expected: FAIL — module not found.

**Step 5: Write `ui/src/hooks/useTheme.ts`**

```ts
import { useState, useEffect, useCallback } from "react";

type Theme = "light" | "dark";

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(() => {
    const saved = localStorage.getItem("evidra-theme");
    if (saved === "light" || saved === "dark") return saved;
    if (typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches) {
      return "dark";
    }
    return "light";
  });

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("evidra-theme", theme);
  }, [theme]);

  const toggle = useCallback(() => {
    setThemeState((prev) => (prev === "dark" ? "light" : "dark"));
  }, []);

  return { theme, toggle };
}
```

**Step 6: Write `ui/src/components/ThemeToggle.tsx`**

```tsx
import { useTheme } from "../hooks/useTheme";

export function ThemeToggle() {
  const { theme, toggle } = useTheme();
  return (
    <button
      onClick={toggle}
      aria-label="Toggle theme"
      className="bg-transparent border border-border rounded-md w-8 h-8 cursor-pointer text-fg-muted flex items-center justify-center transition-all hover:border-accent hover:text-fg"
    >
      {theme === "dark" ? "\u2600" : "\u263D"}
    </button>
  );
}
```

**Step 7: Run tests**

```bash
cd ui && npx vitest run --reporter=verbose
```
Expected: PASS.

**Step 8: Commit**

```bash
git add ui/
git commit -m "feat(ui): add useTheme hook and ThemeToggle component"
```

---

### Task 4: Add CodeBlock component

**Files:**
- Create: `ui/src/components/CodeBlock.tsx`
- Create: `ui/test/components/CodeBlock.test.tsx`

**Step 1: Write the failing test**

`ui/test/components/CodeBlock.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { CodeBlock } from "../../src/components/CodeBlock";

describe("CodeBlock", () => {
  beforeEach(() => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("renders code content", () => {
    render(<CodeBlock code="echo hello" />);
    expect(screen.getByText("echo hello")).toBeInTheDocument();
  });

  it("copies code to clipboard on button click", async () => {
    render(<CodeBlock code="echo hello" />);
    const btn = screen.getByRole("button", { name: /copy/i });
    await userEvent.click(btn);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("echo hello");
  });
});
```

**Step 2: Run test to verify it fails**

```bash
cd ui && npx vitest run --reporter=verbose
```
Expected: FAIL.

**Step 3: Write `ui/src/components/CodeBlock.tsx`**

```tsx
import { useState, useCallback } from "react";

interface CodeBlockProps {
  code: string;
  className?: string;
}

export function CodeBlock({ code, className = "" }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [code]);

  return (
    <div className={`relative bg-[var(--color-code-bg,var(--color-bg-alt))] border border-border rounded-[10px] overflow-hidden ${className}`}>
      <div className="flex items-center gap-1.5 px-4 py-2.5 bg-[var(--color-code-header,var(--color-accent-tint))] border-b border-border">
        <span className="w-2 h-2 rounded-full bg-red-400" />
        <span className="w-2 h-2 rounded-full bg-yellow-400" />
        <span className="w-2 h-2 rounded-full bg-emerald-400" />
      </div>
      <button
        onClick={handleCopy}
        aria-label="Copy code"
        className="absolute top-2 right-3 bg-bg-elevated border border-border rounded px-2 py-0.5 cursor-pointer text-xs font-mono text-fg-muted transition-all hover:border-accent hover:text-fg"
      >
        {copied ? "copied!" : "copy"}
      </button>
      <pre className="px-6 py-5 overflow-x-auto font-mono text-sm leading-7 text-fg-body whitespace-pre">
        {code}
      </pre>
    </div>
  );
}
```

**Step 4: Run tests**

```bash
cd ui && npx vitest run --reporter=verbose
```
Expected: PASS.

**Step 5: Commit**

```bash
git add ui/
git commit -m "feat(ui): add CodeBlock component with copy button"
```

---

### Task 5: Add MermaidDiagram component

**Files:**
- Create: `ui/src/components/MermaidDiagram.tsx`
- Modify: `ui/package.json` (add mermaid)

**Step 1: Install mermaid**

```bash
cd ui && npm install mermaid
```

**Step 2: Write `ui/src/components/MermaidDiagram.tsx`**

```tsx
import { useEffect, useRef, useId } from "react";
import mermaid from "mermaid";

interface MermaidDiagramProps {
  chart: string;
  className?: string;
}

export function MermaidDiagram({ chart, className = "" }: MermaidDiagramProps) {
  const ref = useRef<HTMLDivElement>(null);
  const id = useId().replace(/:/g, "_");

  useEffect(() => {
    if (!ref.current) return;

    const theme = document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "default";
    mermaid.initialize({ startOnLoad: false, theme, securityLevel: "loose" });

    mermaid.render(`mermaid-${id}`, chart).then(({ svg }) => {
      if (ref.current) ref.current.innerHTML = svg;
    });
  }, [chart, id]);

  return (
    <div
      ref={ref}
      className={`flex justify-center bg-bg-elevated border border-border rounded-[10px] p-8 overflow-x-auto ${className}`}
    />
  );
}
```

**Step 3: Verify it builds**

```bash
cd ui && npx vite build
```
Expected: builds successfully.

**Step 4: Commit**

```bash
git add ui/
git commit -m "feat(ui): add MermaidDiagram component"
```

---

### Task 6: Add Layout component (Header + Footer + StatusBar)

**Files:**
- Create: `ui/src/components/Layout.tsx`

**Step 1: Write `ui/src/components/Layout.tsx`**

```tsx
import { ThemeToggle } from "./ThemeToggle";

interface LayoutProps {
  children: React.ReactNode;
}

const NAV_LINKS = [
  { href: "#features", label: "Features" },
  { href: "#architecture", label: "Architecture" },
  { href: "#get-started", label: "Get Started" },
  { href: "#api", label: "API" },
  { href: "#guides", label: "Guides" },
];

export function Layout({ children }: LayoutProps) {
  return (
    <>
      <Header />
      {children}
      <StatusBar />
      <Footer />
    </>
  );
}

function Header() {
  return (
    <header className="sticky top-0 z-50 bg-[color-mix(in_srgb,var(--color-bg)_85%,transparent)] backdrop-blur-xl border-b border-border-subtle">
      <div className="max-w-[980px] mx-auto px-8 flex justify-between items-center py-3">
        <div className="flex items-center gap-8">
          <span className="font-extrabold text-[1.05rem] text-fg tracking-tight">
            evidra<span className="text-accent">.</span>
          </span>
          <nav className="flex gap-6">
            {NAV_LINKS.map((l) => (
              <a
                key={l.href}
                href={l.href}
                className="text-[0.82rem] font-medium text-fg-muted tracking-wide hover:text-fg no-underline transition-colors"
              >
                {l.label}
              </a>
            ))}
          </nav>
        </div>
        <div className="flex items-center gap-3">
          <a
            className="inline-flex items-center gap-1.5 text-[0.8rem] font-medium text-fg-muted px-3 py-1.5 border border-border rounded-md transition-all hover:border-accent hover:text-fg no-underline"
            href="https://github.com/vitas/evidra"
            target="_blank"
            rel="noopener"
          >
            <svg viewBox="0 0 16 16" className="w-4 h-4 fill-current">
              <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
            </svg>
            GitHub
          </a>
          <ThemeToggle />
        </div>
      </div>
    </header>
  );
}

function StatusBar() {
  return (
    <div className="border-t border-border-subtle py-2.5 text-[0.78rem] text-fg-muted font-mono">
      <div className="max-w-[980px] mx-auto px-8 flex justify-center gap-10 items-center flex-wrap">
        <StatusDot endpoint="/healthz" label="api" />
        <StatusDot endpoint="/readyz" label="database" />
      </div>
    </div>
  );
}

function StatusDot({ endpoint, label }: { endpoint: string; label: string }) {
  // Status polling is handled via useEffect in the real implementation.
  // For now, render "checking" state.
  return (
    <div className="flex items-center gap-1.5">
      <span className="w-[7px] h-[7px] rounded-full bg-fg-muted inline-block" />
      {label}: <span>checking</span>
    </div>
  );
}

function Footer() {
  return (
    <footer className="py-8 text-center text-[0.8rem] text-fg-muted border-t border-border-subtle">
      <div className="max-w-[980px] mx-auto px-8">
        <a href="https://github.com/vitas/evidra" target="_blank" rel="noopener" className="text-fg-muted font-medium hover:text-accent">
          github.com/vitas/evidra
        </a>
        {" \u00B7 Apache 2.0"}
      </div>
    </footer>
  );
}
```

**Step 2: Verify it builds**

```bash
cd ui && npx vite build
```
Expected: builds successfully.

**Step 3: Commit**

```bash
git add ui/
git commit -m "feat(ui): add Layout with Header, StatusBar, and Footer"
```

---

### Task 7: Build the Landing page

**Files:**
- Create: `ui/src/pages/Landing.tsx`
- Modify: `ui/src/App.tsx`

**Step 1: Write `ui/src/pages/Landing.tsx`**

This is the main page with all sections. Port the content from the current `index.html`.

```tsx
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
  { icon: "\u25A4", title: "Measure", desc: "Detect 7 behavioral signals: retry loops, artifact drift, protocol violations, blast radius, and more." },
  { icon: "\u2605", title: "Score", desc: "Compute reliability scorecards with weighted penalties, safety floors, and confidence levels." },
  { icon: "\u21C4", title: "Compare", desc: "Compare actors, tools, and scopes side-by-side with workload overlap analysis." },
];

const GUIDES = [
  { tag: "CI / CD", title: "CI Integration", desc: "Add reliability scoring to your CI pipeline — Terraform, Kubernetes, Docker, and more.", href: "https://github.com/vitas/evidra/blob/main/docs/guides/terraform-ci-quickstart.md" },
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
      <Guides />
    </>
  );
}

function Divider() {
  return <hr className="h-px border-none bg-gradient-to-r from-transparent via-accent to-transparent m-0" />;
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <div className="font-mono text-[0.72rem] font-medium tracking-widest uppercase text-accent mb-3">{children}</div>;
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h2 className="text-[1.6rem] font-bold text-fg tracking-tight mb-2">{children}</h2>;
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
          <a href="#get-started" className="inline-flex items-center gap-1.5 px-5 py-2.5 rounded-lg text-[0.88rem] font-semibold bg-accent text-white transition-all hover:bg-accent-bright hover:-translate-y-0.5 hover:shadow-lg no-underline dark:bg-[#065f46] dark:text-[#d1fae5] dark:hover:bg-[#047857]">
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
        <p className="text-fg-muted mb-10 text-[0.95rem]">A flight recorder for infrastructure automation &mdash; observe, measure, score, compare.</p>
        <div className="grid grid-cols-4 gap-5 max-md:grid-cols-2 max-sm:grid-cols-1">
          {FEATURES.map((f) => (
            <div key={f.title} className="bg-bg-elevated border border-border-subtle border-l-[3px] border-l-accent rounded-lg p-6 shadow-sm transition-all hover:shadow-md hover:-translate-y-0.5">
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
  const [tab, setTab] = useState<"pipeline" | "system">("pipeline");
  return (
    <section id="architecture" className="py-14 bg-bg-alt">
      <Container>
        <SectionLabel>Architecture</SectionLabel>
        <SectionTitle>How It All Fits Together</SectionTitle>
        <p className="text-fg-muted mb-10 text-[0.95rem]">Two views: how evidence flows through the scoring pipeline, and how components are deployed.</p>
        <div className="inline-flex bg-accent-subtle border border-border rounded-lg p-[3px] mb-6">
          <TabBtn active={tab === "pipeline"} onClick={() => setTab("pipeline")}>How It Works</TabBtn>
          <TabBtn active={tab === "system"} onClick={() => setTab("system")}>System Architecture</TabBtn>
        </div>
        {tab === "pipeline" && <MermaidDiagram chart={PIPELINE_CHART} />}
        {tab === "system" && <MermaidDiagram chart={SYSTEM_CHART} />}
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
        <p className="text-fg-muted mb-10 text-[0.95rem]">Up and running in under 5 minutes.</p>
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
        <p className="text-fg-muted mb-10 text-[0.95rem]">Full OpenAPI 3.0 documentation for all endpoints.</p>
        <a href="/docs/api" className="flex items-center justify-between bg-bg-elevated border border-border rounded-[10px] p-6 px-8 shadow-sm transition-all hover:shadow-md hover:border-accent no-underline">
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

function Guides() {
  return (
    <section id="guides" className="py-14">
      <Container>
        <SectionLabel>Guides</SectionLabel>
        <SectionTitle>Integrate Into Your Workflow</SectionTitle>
        <p className="text-fg-muted mb-10 text-[0.95rem]">Step-by-step guides for common integration patterns.</p>
        <div className="grid grid-cols-3 gap-5 max-md:grid-cols-1">
          {GUIDES.map((g) => (
            <a key={g.title} href={g.href} target="_blank" rel="noopener" className="bg-bg-elevated border border-border-subtle rounded-[10px] p-6 shadow-sm transition-all hover:shadow-md hover:border-accent hover:-translate-y-0.5 no-underline block">
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
        active ? "bg-bg-elevated text-fg shadow-sm" : "bg-transparent text-fg-muted hover:text-fg"
      }`}
    >
      {children}
    </button>
  );
}
```

**Step 2: Update `ui/src/App.tsx`**

```tsx
import { Layout } from "./components/Layout";
import { Landing } from "./pages/Landing";

export function App() {
  return (
    <Layout>
      <Landing />
    </Layout>
  );
}
```

**Step 3: Verify it builds**

```bash
cd ui && npx vite build
```
Expected: builds successfully.

**Step 4: Commit**

```bash
git add ui/
git commit -m "feat(ui): add Landing page with all sections"
```

---

### Task 8: Add StatusBar health polling

**Files:**
- Create: `ui/src/hooks/useHealthCheck.ts`
- Modify: `ui/src/components/Layout.tsx` (update StatusBar)

**Step 1: Write `ui/src/hooks/useHealthCheck.ts`**

```ts
import { useState, useEffect } from "react";

type Status = "checking" | "healthy" | "unhealthy";

export function useHealthCheck(endpoint: string, intervalMs = 30000) {
  const [status, setStatus] = useState<Status>("checking");

  useEffect(() => {
    const check = () => {
      fetch(endpoint)
        .then((r) => setStatus(r.ok ? "healthy" : "unhealthy"))
        .catch(() => setStatus("unhealthy"));
    };
    check();
    const id = setInterval(check, intervalMs);
    return () => clearInterval(id);
  }, [endpoint, intervalMs]);

  return status;
}
```

**Step 2: Update `StatusBar` in `Layout.tsx`**

Replace the `StatusDot` and `StatusBar` functions to use `useHealthCheck`:

```tsx
import { useHealthCheck } from "../hooks/useHealthCheck";

function StatusBar() {
  return (
    <div className="border-t border-border-subtle py-2.5 text-[0.78rem] text-fg-muted font-mono">
      <div className="max-w-[980px] mx-auto px-8 flex justify-center gap-10 items-center flex-wrap">
        <StatusDot endpoint="/healthz" label="api" />
        <StatusDot endpoint="/readyz" label="database" />
      </div>
    </div>
  );
}

function StatusDot({ endpoint, label }: { endpoint: string; label: string }) {
  const status = useHealthCheck(endpoint);
  const dotColor = status === "healthy" ? "bg-accent shadow-[0_0_4px_rgba(5,150,105,0.4)]"
    : status === "unhealthy" ? "bg-red-400 shadow-[0_0_4px_rgba(248,113,113,0.4)]"
    : "bg-fg-muted";
  const text = status === "healthy" ? (label === "api" ? "healthy" : "connected")
    : status === "unhealthy" ? (label === "api" ? "unreachable" : "unavailable")
    : "checking";

  return (
    <div className="flex items-center gap-1.5">
      <span className={`w-[7px] h-[7px] rounded-full inline-block transition-colors ${dotColor}`} />
      {label}: <span>{text}</span>
    </div>
  );
}
```

**Step 3: Verify it builds**

```bash
cd ui && npx vite build
```
Expected: builds successfully.

**Step 4: Commit**

```bash
git add ui/
git commit -m "feat(ui): add health check polling to StatusBar"
```

---

### Task 9: Wire Go embedding with build tag

**Files:**
- Create: `uiembed.go` (project root)
- Create: `uiembed_embed.go` (project root)
- Modify: `cmd/evidra-api/main.go` (use UIDistFS instead of local embed)
- Modify: `internal/api/router.go` (add SPA fallback)
- Create: `internal/api/ui_handler.go`

**Step 1: Write `uiembed.go`** (project root)

```go
package evidrabenchmark

import "io/fs"

// UIDistFS is the embedded UI filesystem. It is nil when the binary is built
// without the embed_ui tag (e.g. during tests or non-UI builds).
//
// Build with UI:
//
//	cd ui && npm run build
//	cd .. && go build -tags embed_ui -o bin/evidra-api ./cmd/evidra-api
var UIDistFS fs.FS
```

**Step 2: Write `uiembed_embed.go`** (project root)

```go
//go:build embed_ui

package evidrabenchmark

import (
	"embed"
	"io/fs"
)

//go:embed all:ui/dist
var uiDistRaw embed.FS

func init() {
	sub, err := fs.Sub(uiDistRaw, "ui/dist")
	if err != nil {
		panic("uiembed: " + err.Error())
	}
	UIDistFS = sub
}
```

**Step 3: Write `internal/api/ui_handler.go`**

```go
package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// uiHandler serves the embedded UI filesystem with SPA fallback.
// Unknown paths fall back to index.html for client-side routing.
func uiHandler(uiFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(uiFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		f, err := uiFS.Open(path)
		if err != nil {
			// SPA fallback: serve index.html for unknown paths.
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, r)
	})
}
```

**Step 4: Update `cmd/evidra-api/main.go`**

Remove the `//go:embed static` and `staticFS` variable. Replace the UIFS setup:

Before:
```go
//go:embed static
var staticFS embed.FS

// ... in run():
uiFS, err := fs.Sub(staticFS, "static")
if err != nil {
    log.Fatalf("embed static: %v", err)
}
cfg.UIFS = uiFS
```

After:
```go
// ... in run():
// UI: prefer embedded React build (embed_ui tag), fall back to static files.
if evidrabenchmark.UIDistFS != nil {
    cfg.UIFS = evidrabenchmark.UIDistFS
} else {
    uiFS, err := fs.Sub(staticFS, "static")
    if err != nil {
        log.Fatalf("embed static: %v", err)
    }
    cfg.UIFS = uiFS
}
```

Keep the `//go:embed static` as fallback for builds without the tag (serves Swagger UI + openapi.yaml).

**Step 5: Update `internal/api/router.go`**

Replace the file server line with the SPA handler:

Before:
```go
if cfg.UIFS != nil {
    mux.Handle("/", http.FileServer(http.FS(cfg.UIFS)))
}
```

After:
```go
if cfg.UIFS != nil {
    mux.Handle("/", uiHandler(cfg.UIFS))
}
```

**Step 6: Run tests**

```bash
go test ./internal/api/... -v -count=1
```
Expected: all tests pass (existing landing_test.go still works).

**Step 7: Verify Go build compiles**

```bash
go build ./cmd/evidra-api
```
Expected: compiles (without UI — UIDistFS is nil, falls back to static/).

**Step 8: Commit**

```bash
git add uiembed.go uiembed_embed.go internal/api/ui_handler.go cmd/evidra-api/main.go internal/api/router.go
git commit -m "feat(api): add go:embed with build tag for React UI"
```

---

### Task 10: Update Makefile and add .gitignore for UI build artifacts

**Files:**
- Modify: `Makefile`
- Create: `ui/.gitignore`

**Step 1: Write `ui/.gitignore`**

```
node_modules/
dist/
*.tsbuildinfo
```

**Step 2: Update `Makefile`** — add ui-build and build-api targets

Add these targets:

```makefile
.PHONY: ui-build build-api

ui-build:
	cd ui && npm ci && npm run build

build-api: ui-build
	go build -tags embed_ui -o bin/evidra-api ./cmd/evidra-api
```

**Step 3: Verify full build pipeline**

```bash
make build-api
```
Expected: UI builds, then Go binary compiles with embedded React app.

**Step 4: Commit**

```bash
git add ui/.gitignore Makefile
git commit -m "chore: add ui-build and build-api Makefile targets"
```

---

### Task 11: Add Vitest smoke test for App component

**Files:**
- Create: `ui/test/components/App.test.tsx`

**Step 1: Write the test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect, beforeEach } from "vitest";
import { App } from "../../src/App";

describe("App", () => {
  beforeEach(() => {
    document.documentElement.setAttribute("data-theme", "light");
    localStorage.clear();
  });

  it("renders the landing page with hero heading", () => {
    render(<App />);
    expect(screen.getByText(/Reliability Scoring for/i)).toBeInTheDocument();
  });

  it("renders navigation links", () => {
    render(<App />);
    expect(screen.getByText("Features")).toBeInTheDocument();
    expect(screen.getByText("Architecture")).toBeInTheDocument();
    expect(screen.getByText("Get Started")).toBeInTheDocument();
  });
});
```

**Step 2: Run tests**

```bash
cd ui && npx vitest run --reporter=verbose
```
Expected: all tests pass.

**Step 3: Commit**

```bash
git add ui/test/
git commit -m "test(ui): add App smoke test"
```

---

### Task 12: Visual verification

**Step 1: Start dev server**

```bash
cd ui && npm run dev
```

**Step 2: Open in browser and verify**

- Visit http://localhost:5173
- Verify all sections render correctly
- Toggle light/dark theme
- Click tab buttons (Architecture, Getting Started)
- Verify Mermaid diagrams render
- Verify code block copy works
- Test responsive layout (resize browser)

**Step 3: Fix any visual issues found**

**Step 4: Final commit if any fixes made**

```bash
git add -u
git commit -m "fix(ui): visual refinements from manual testing"
```
