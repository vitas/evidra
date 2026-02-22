import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "./App";

describe("App deep link", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    window.history.replaceState(
      {},
      "",
      "/ui/explorer/change/chg_demo?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
    );
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("loads change detail for URL-provided change id", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.includes("/v1/subjects")) {
        return jsonResponse({
          items: [{ subject: "payments-api", namespace: "prod-eu", cluster: "eu-1" }],
        });
      }
      if (url.includes("/v1/changes?") && !url.includes("/v1/changes/chg_demo")) {
        return jsonResponse({
          items: [
            {
              id: "chg_demo",
              subject: "payments-api",
              application: "payments-api",
              target_cluster: "eu-1",
              namespace: "prod-eu",
              primary_provider: "argo",
              result_status: "succeeded",
              health_status: "healthy",
              started_at: "2026-02-16T12:00:00Z",
              completed_at: "2026-02-16T12:02:00Z",
              event_count: 1,
            },
          ],
          page: {
            next_cursor: "",
          },
        });
      }
      if (url.includes("/v1/changes/chg_demo/timeline")) {
        return jsonResponse({
          items: [
            {
              id: "evt_1",
              source: "argocd",
              type: "argo.sync.finished",
              time: "2026-02-16T12:02:00Z",
              subject: "payments-api",
              extensions: { status: "Succeeded" },
            },
          ],
        });
      }
      if (url.includes("/v1/changes/chg_demo/evidence")) {
        return jsonResponse({
          change: {
            id: "chg_demo",
          },
          supporting_observations: [],
        });
      }
      if (url.includes("/v1/changes/chg_demo")) {
        return jsonResponse({
          id: "chg_demo",
          subject: "payments-api",
          application: "payments-api",
          target_cluster: "eu-1",
          namespace: "prod-eu",
          primary_provider: "argo",
          result_status: "succeeded",
          health_status: "healthy",
          started_at: "2026-02-16T12:00:00Z",
          completed_at: "2026-02-16T12:02:00Z",
          event_count: 1,
        });
      }
      throw new Error(`unexpected fetch url: ${url}`);
    });

    globalThis.fetch = fetchMock as unknown as typeof fetch;

    render(<App />);

    await screen.findByTestId("export-change-button");
    await screen.findByTestId("root-cause-banner");
    await screen.findByTestId("change-permalink");

    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((c) => String(c[0]));
      expect(urls.some((u) => u.includes("/v1/changes/chg_demo"))).toBe(true);
    });
  });

  it("does not override deep-link subject with first subject from API list", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.includes("/v1/subjects")) {
        return jsonResponse({
          items: [
            { subject: "orders-api", namespace: "prod-us", cluster: "us-1" },
            { subject: "payments-api", namespace: "prod-eu", cluster: "eu-1" },
          ],
        });
      }
      if (url.includes("/v1/changes?")) {
        const parsed = new URL(url, "http://localhost");
        if (parsed.searchParams.get("subject") !== "payments-api:prod-eu:eu-1") {
          throw new Error(`unexpected subject in changes request: ${parsed.searchParams.get("subject")}`);
        }
        return jsonResponse({
          items: [
            {
              id: "chg_demo",
              subject: "payments-api",
              application: "payments-api",
              target_cluster: "eu-1",
              namespace: "prod-eu",
              primary_provider: "argo",
              result_status: "succeeded",
              health_status: "healthy",
              started_at: "2026-02-16T12:00:00Z",
              completed_at: "2026-02-16T12:02:00Z",
              event_count: 1,
            },
          ],
        });
      }
      if (url.includes("/v1/changes/chg_demo/timeline")) {
        return jsonResponse({ items: [] });
      }
      if (url.includes("/v1/changes/chg_demo/evidence")) {
        return jsonResponse({ change: { id: "chg_demo" }, supporting_observations: [] });
      }
      if (url.includes("/v1/changes/chg_demo")) {
        return jsonResponse({
          id: "chg_demo",
          subject: "payments-api",
          application: "payments-api",
          target_cluster: "eu-1",
          namespace: "prod-eu",
          primary_provider: "argo",
          result_status: "succeeded",
          health_status: "healthy",
          started_at: "2026-02-16T12:00:00Z",
          completed_at: "2026-02-16T12:02:00Z",
          event_count: 1,
        });
      }
      throw new Error(`unexpected fetch url: ${url}`);
    });

    globalThis.fetch = fetchMock as unknown as typeof fetch;

    render(<App />);

    await screen.findByTestId("changes-list");
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((c) => String(c[0]));
      expect(
        urls.some(
          (u) =>
            u.includes("/v1/changes?") &&
            u.includes("subject=payments-api%3Aprod-eu%3Aeu-1"),
        ),
      ).toBe(true);
    });
  });

  it("renders long subject in structured two-line format without raw cluster URL", async () => {
    window.history.replaceState({}, "", "/ui/");

    const longApp = "guestbook-demo-with-very-long-application-name-for-selector";
    const longSubject = `${longApp}:demo:https://kubernetes.default.svc`;
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.includes("/v1/subjects")) {
        return jsonResponse({
          items: [
            { subject: longApp, namespace: "demo", cluster: "https://kubernetes.default.svc" },
            { subject: "payments-api", namespace: "prod-eu", cluster: "eu-1" },
          ],
        });
      }
      if (url.includes("/v1/changes?")) {
        const parsed = new URL(url, "http://localhost");
        if (parsed.searchParams.get("subject") !== longSubject) {
          throw new Error(`unexpected subject in changes request: ${parsed.searchParams.get("subject")}`);
        }
        return jsonResponse({ items: [] });
      }
      throw new Error(`unexpected fetch url: ${url}`);
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    render(<App />);

    const select = await screen.findByTestId("subject-select");
    expect(select).toHaveTextContent(longApp);
    expect(select).toHaveTextContent("demo · in-cluster");
    expect(select).not.toHaveTextContent("https://kubernetes.default.svc");

    fireEvent.click(select.querySelector("button") as HTMLButtonElement);
    await screen.findByTestId("subject-search");
    expect(screen.getAllByText(longApp).length).toBeGreaterThan(0);
    expect(screen.getAllByText("demo · in-cluster").length).toBeGreaterThan(0);
    expect(screen.queryByText("https://kubernetes.default.svc")).not.toBeInTheDocument();
  });
});

function jsonResponse(payload: unknown): Response {
  return new Response(JSON.stringify(payload), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
