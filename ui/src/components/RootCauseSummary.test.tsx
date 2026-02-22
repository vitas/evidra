import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ChangeDetail, EventItem } from "../types";
import { RootCauseBanner } from "./RootCauseBanner";

const change: ChangeDetail = {
  id: "chg_demo",
  subject: "payments-api",
  application: "payments-api",
  target_cluster: "eu-1",
  namespace: "prod-eu",
  primary_provider: "argo",
  result_status: "failed",
  health_status: "degraded",
  started_at: "2026-02-16T12:00:00Z",
  completed_at: "2026-02-16T12:02:00Z",
  event_count: 2,
  revision: "abc123",
  external_change_id: "CHG-7",
  ticket_id: "JIRA-7",
};

const events: EventItem[] = [
  {
    id: "evt_1",
    source: "argocd",
    type: "argo.sync.finished",
    time: "2026-02-16T12:02:00Z",
    subject: "payments-api",
    extensions: { status: "Failed" },
  },
];

describe("RootCauseBanner", () => {
  it("renders status, root-cause text, correlators, and actions", () => {
    const onCopy = vi.fn();
    const onExport = vi.fn();

    render(
      <RootCauseBanner
        change={change}
        events={events}
        permalink="http://localhost:8080/ui/explorer/change/chg_demo"
        onCopyPermalink={onCopy}
        onExport={onExport}
      />,
    );

    expect(screen.getByTestId("root-cause-banner")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
    expect(screen.getByTestId("root-cause-text").textContent?.toLowerCase()).toContain("root cause");
    expect(screen.getAllByText(/abc123/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/CHG-7/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/JIRA-7/).length).toBeGreaterThan(0);

    fireEvent.click(screen.getByTestId("copy-permalink-button"));
    fireEvent.click(screen.getByTestId("export-change-button"));

    expect(onCopy).toHaveBeenCalledTimes(1);
    expect(onExport).toHaveBeenCalledTimes(1);
  });
});
