import { render, screen, within } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import mermaid from "mermaid";

vi.mock("../../src/components/MermaidDiagram", () => ({
  MermaidDiagram: ({ chart }: { chart: string }) => (
    <div data-testid="mermaid-diagram">{chart}</div>
  ),
}));

vi.mock("../../src/hooks/useHealthCheck", () => ({
  useHealthCheck: () => "healthy",
}));

import { App } from "../../src/App";
import { SEQUENCE_CHART } from "../../src/pages/Landing";

describe("App", () => {
  beforeEach(() => {
    document.documentElement.setAttribute("data-theme", "light");
    localStorage.clear();
    // BrowserRouter reads window.location; default to "/".
    window.history.pushState({}, "", "/");
  });

  it("renders the landing page with hero heading", () => {
    render(<App />);
    expect(
      screen.getByRole("heading", {
        name: /Know what your agent intended\.\s*Know what actually happened\./i,
      }),
    ).toBeInTheDocument();
  });

  it("renders navigation links", () => {
    render(<App />);
    const nav = screen.getByRole("navigation");
    expect(nav).toBeInTheDocument();
    expect(nav.querySelector('a[href="#features"]')).toHaveTextContent(
      "Features",
    );
    expect(nav.querySelector('a[href="#architecture"]')).toHaveTextContent(
      "Architecture",
    );
    expect(nav.querySelector('a[href="#get-started"]')).toHaveTextContent(
      "Get Started",
    );
  });

  it("renders a Bench CTA in the hero actions", () => {
    render(<App />);

    const hero = screen
      .getByRole("heading", {
        name: /Know what your agent intended\.\s*Know what actually happened\./i,
      })
      .closest("section");

    expect(hero).not.toBeNull();
    expect(
      within(hero as HTMLElement).getByRole("link", {
        name: "Test Your Agent Skills",
      }),
    ).toHaveAttribute("href", "https://lab.evidra.cc");
  });

  it("does not expose raw signal weights on the landing page", () => {
    render(<App />);

    expect(screen.queryByText("0.30")).not.toBeInTheDocument();
    expect(screen.queryByText("0.25")).not.toBeInTheDocument();
    expect(screen.queryByText("0.15")).not.toBeInTheDocument();
    expect(screen.queryByText("−0.05")).not.toBeInTheDocument();
  });

  it("describes prescribe output as risk_inputs and effective_risk", () => {
    render(<App />);

    expect(screen.getByText(/risk_inputs/i)).toBeInTheDocument();
    expect(screen.getByText(/effective_risk/i)).toBeInTheDocument();
  });

  it("uses a valid mermaid sequence chart for the default protocol flow", async () => {
    await expect(mermaid.parse(SEQUENCE_CHART)).resolves.toBeTruthy();
  });
});
