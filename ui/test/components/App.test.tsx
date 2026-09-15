import { render, screen, within } from "@testing-library/react";
import { describe, it, expect, beforeEach } from "vitest";
import { App } from "../../src/App";

describe("App", () => {
  beforeEach(() => {
    document.documentElement.setAttribute("data-theme", "light");
    localStorage.clear();
  });

  it("leads with the OSS Core outcome", () => {
    render(<App />);
    expect(
      screen.getByRole("heading", {
        name: /Verifiable evidence from the MCP execution boundary/i,
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/agents actually did/i)).not.toBeInTheDocument();
  });

  it("makes Core getting started the primary action", () => {
    render(<App />);
    expect(
      screen.getByRole("link", { name: /Get started/i }),
    ).toHaveAttribute(
      "href",
      "https://github.com/vitas/evidra/blob/main/docs/getting-started.md",
    );
  });

  it("shows the three source boundaries", () => {
    render(<App />);
    expect(screen.getByRole("heading", { name: "Declared" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Observed" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Reported" })).toBeInTheDocument();
  });

  it("does not expose removed hosted-product navigation", () => {
    render(<App />);
    expect(screen.queryByText(/Dashboard Access/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Get API Key/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/Reliability Dashboard/i)).not.toBeInTheDocument();
  });

  it("keeps Bench secondary", () => {
    render(<App />);
    const bench = screen.getByRole("link", { name: /Evidra Bench/i });
    expect(bench).toHaveAttribute("href", "https://github.com/vitas/evidra-bench");
    expect(bench).not.toHaveAttribute("data-primary", "true");
  });
});

describe("App — Core content contract", () => {
  beforeEach(() => {
    document.documentElement.setAttribute("data-theme", "light");
    localStorage.clear();
    render(<App />);
  });

  it("shows the runtime sequence Agent → Evidra → Upstream MCP server", () => {
    expect(
      screen.getByRole("heading", { name: /Runtime topology/i }),
    ).toBeInTheDocument();
    const topology = document.getElementById("runtime-topology");
    expect(topology).not.toBeNull();
    expect(topology).toHaveTextContent("Agent");
    expect(topology).toHaveTextContent(/Evidra MCP endpoint/i);
    expect(topology).toHaveTextContent(/Upstream MCP server/i);
  });

  it("exposes the complete runtime topology as one accessible image", () => {
    expect(
      screen.getByRole("img", {
        name: "Agent connects to the Evidra MCP endpoint, which forwards calls to one upstream MCP server and writes a signed evidence directory.",
      }),
    ).toBeInTheDocument();
  });

  it("keeps Docs navigation in the current tab", () => {
    expect(screen.getByRole("link", { name: "Docs" })).not.toHaveAttribute(
      "target",
    );
  });

  it("states that a successful tool response is not outcome proof", () => {
    expect(
      screen.getByText(/successful tool response is not proof of the external outcome/i),
    ).toBeInTheDocument();
  });

  it("states the limits of recording coverage", () => {
    expect(
      screen.queryByText(/guarantees is that the execution is recorded/i),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/unknown loss can still exist/i)).toBeInTheDocument();
  });

  it("separates generated reconciliation from human judgment", () => {
    const example = document.getElementById("reconciliation-example");
    expect(example).not.toBeNull();
    const reconciliation = within(example as HTMLElement);

    expect(
      reconciliation.getByText(
        /restart_deployment → success; get_status → success; result fingerprint present/i,
      ),
    ).toBeInTheDocument();
    expect(
      reconciliation.getByText("Reviewer interpretation"),
    ).toBeInTheDocument();
    expect(
      reconciliation.getByText(
        /Evidra generates the reconciliation summary from the recorded chain; a human judges what it means for the external outcome/i,
      ),
    ).toBeInTheDocument();
    expect(
      reconciliation.queryByText(/get_status → ready/i),
    ).not.toBeInTheDocument();
  });

  it("shows only current commands", () => {
    expect(screen.getByText(/evidra-mcp --proxy/)).toBeInTheDocument();
    expect(screen.getByText(/evidra summarize --dir/)).toBeInTheDocument();
    expect(screen.getByText(/evidra verify --dir/)).toBeInTheDocument();
  });

  it("names the three supported use cases", () => {
    expect(
      screen.getByRole("heading", { name: "Incident reconstruction" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Agent evaluation" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Operational review" }),
    ).toBeInTheDocument();
  });

  it("has no hosted-product CTAs or features", () => {
    expect(
      screen.queryByRole("link", { name: /^Hosted/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /Start with Bench/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: /Scorecard/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: /Signals\b/i }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/API key/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/database status/i)).not.toBeInTheDocument();
  });
});
