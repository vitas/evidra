import { ThemeToggle } from "./ThemeToggle";

interface LayoutProps {
  children: React.ReactNode;
}

const NAV_LINKS = [
  { href: "#runtime-topology", label: "Product" },
  { href: "https://github.com/vitas/evidra/blob/main/docs/getting-started.md", label: "Docs", external: false },
];

export function Layout({ children }: LayoutProps) {
  return (
    <>
      <a href="#main-content" className="skip-link">
        Skip to content
      </a>
      <Header />
      <main id="main-content">{children}</main>
      <Footer />
    </>
  );
}

function Header() {
  return (
    <header className="border-b border-border-subtle bg-bg">
      <div className="mx-auto max-w-[72rem] px-4 flex flex-wrap items-center gap-x-6 gap-y-2 py-3 justify-between">
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
          <a
            href="/"
            className="font-bold text-[1.05rem] text-fg no-underline hover:text-fg"
          >
            evidra<span className="text-accent">.</span>
          </a>
          <nav aria-label="Main" className="flex flex-wrap items-center gap-x-5 gap-y-1">
            {NAV_LINKS.map((l) => (
              <a
                key={l.label}
                href={l.href}
                {...(l.external ? { target: "_blank", rel: "noopener" } : {})}
                className="text-[0.85rem] font-medium text-fg-muted no-underline hover:text-fg"
              >
                {l.label}
              </a>
            ))}
            <a
              href="https://github.com/vitas/evidra"
              target="_blank"
              rel="noopener"
              className="text-[0.85rem] font-medium text-fg-muted no-underline hover:text-fg"
            >
              GitHub
            </a>
          </nav>
        </div>
        <ThemeToggle />
      </div>
    </header>
  );
}

function Footer() {
  return (
    <footer className="border-t border-border-subtle py-8 mt-8">
      <div className="mx-auto max-w-[72rem] px-4 flex flex-wrap items-center gap-x-6 gap-y-2 text-[0.85rem] text-fg-muted">
        <a
          href="https://github.com/vitas/evidra"
          target="_blank"
          rel="noopener"
          className="no-underline hover:text-fg"
        >
          GitHub
        </a>
        <a
          href="https://github.com/vitas/evidra/blob/main/docs/getting-started.md"
          target="_blank"
          rel="noopener"
          className="no-underline hover:text-fg"
        >
          Documentation
        </a>
        <a
          href="https://github.com/vitas/evidra/blob/main/SECURITY.md"
          target="_blank"
          rel="noopener"
          className="no-underline hover:text-fg"
        >
          Security
        </a>
        <a
          href="https://github.com/vitas/evidra-bench"
          target="_blank"
          rel="noopener"
          className="no-underline hover:text-fg"
        >
          Evidra Bench
        </a>
        <span>Apache 2.0</span>
      </div>
    </footer>
  );
}
