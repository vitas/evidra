# React Landing Page Design

## Goal

Replace the plain HTML landing page (`cmd/evidra-api/static/index.html`) with a React SPA using the same stack conventions as evidra-mcp, upgraded to current versions. Structure for future SaaS expansion.

## Stack

| Aspect | Choice |
|--------|--------|
| Framework | React 19 |
| Language | TypeScript 5 (strict) |
| Bundler | Vite 6 |
| Styling | Tailwind v4 (CSS-first config) |
| Fonts | Plus Jakarta Sans, JetBrains Mono (Google Fonts) |
| Unit Tests | Vitest + Testing Library |
| E2E Tests | Playwright |
| Diagrams | Mermaid (npm package) |
| Routing | Hash-based (no library) |
| Go Embedding | `go:embed ui/dist` with `-tags embed_ui` |

No shadcn/ui, no CSS-in-JS, no router library.

## Project Structure

```
ui/
├── index.html
├── package.json
├── vite.config.ts
├── tsconfig.json
├── src/
│   ├── main.tsx
│   ├── App.tsx                  # Hash router (landing only for now)
│   ├── components/
│   │   ├── Layout.tsx           # Header + footer + status bar
│   │   ├── ThemeToggle.tsx      # Light/dark toggle
│   │   ├── CodeBlock.tsx        # Code display + copy
│   │   └── MermaidDiagram.tsx   # Mermaid wrapper
│   ├── pages/
│   │   └── Landing.tsx          # All landing sections
│   ├── hooks/
│   │   └── useTheme.ts          # Theme + localStorage
│   └── styles/
│       └── global.css           # Tailwind directives + tokens
├── test/
│   ├── setup.ts
│   └── components/
└── e2e/
```

## Go Integration

Same pattern as evidra-mcp:

- `uiembed.go` — `var UIDistFS fs.FS` (nil without build tag)
- `uiembed_embed.go` — `//go:build embed_ui`, embeds `ui/dist`
- Router serves `UIFS` filesystem, SPA fallback to index.html
- Swagger UI and openapi.yaml remain as plain static files in `cmd/evidra-api/static/`

## Theme System

- Tailwind v4 CSS-first config with emerald green palette
- `data-theme="light|dark"` on `<html>`, toggled via `useTheme` hook
- CSS variables for theme tokens referenced by Tailwind utilities
- Respects `prefers-color-scheme`, persists choice in localStorage

### Colors

- Light: `#fafffe` base, `#059669` accent, `#d1fae5` tints
- Dark: `#0c0f0e` base, `#34d399` accent, subdued buttons `#065f46`

## Landing Page Sections

1. Header — logo, nav links, GitHub button, theme toggle (sticky, backdrop blur)
2. Hero — eyebrow badge with pulse, heading, tagline, CTA buttons, version tag
3. Features — 4-card grid (Observe, Measure, Score, Compare)
4. Architecture — tabbed Mermaid diagrams (pipeline + system)
5. Getting Started — tabbed code blocks (Binary, Homebrew, Self-Hosted)
6. API Reference — card linking to /docs/api
7. Guides — 3-card grid (CI, Observability, SARIF)
8. Status bar — healthz/readyz polling with dot indicators
9. Footer

## Build Pipeline

```makefile
ui-build:
	cd ui && npm ci && npm run build

build-api: ui-build
	go build -tags embed_ui -o bin/evidra-api ./cmd/evidra-api
```

Dev workflow: `cd ui && npm run dev` with Vite proxy to Go API on :8080.

## Tests

- Vitest smoke test for App component rendering
- Existing Go route tests (`internal/api/landing_test.go`) remain unchanged
- Playwright E2E tests for visual verification

## Out of Scope

- Swagger UI page (stays as plain HTML)
- Additional SaaS pages (dashboard, settings) — future work
- Authentication UI — future work
- shadcn/ui components — add when needed for forms/dialogs
