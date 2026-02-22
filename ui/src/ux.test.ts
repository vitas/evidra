import { describe, expect, it } from "vitest";

import type { ChangeDetail, EventItem } from "./types";
import {
  computeOverallStatus,
  computeTimeToDetect,
  extractCorrelators,
  inferRootCause,
  sortEventsChronologically,
} from "./utils/evidenceSummary";

const change: ChangeDetail = {
  id: "chg_1",
  subject: "payments-api",
  application: "payments-api",
  target_cluster: "eu-1",
  namespace: "prod-eu",
  primary_provider: "argo",
  result_status: "succeeded",
  health_status: "healthy",
  started_at: "2026-02-16T12:00:00Z",
  completed_at: "2026-02-16T12:02:00Z",
  event_count: 2,
  revision: "abc123",
  external_change_id: "CHG-1",
  ticket_id: "JIRA-7",
  approval_reference: "APR-9",
};

const events: EventItem[] = [
  {
    id: "evt_2",
    source: "argocd",
    type: "argo.health.changed",
    time: "2026-02-16T12:03:00Z",
    subject: "payments-api",
    extensions: { health: "Degraded" },
  },
  {
    id: "evt_1",
    source: "argocd",
    type: "argo.sync.finished",
    time: "2026-02-16T12:02:00Z",
    subject: "payments-api",
    extensions: { status: "Succeeded" },
  },
];

describe("evidenceSummary helpers", () => {
  it("sorts events chronologically", () => {
    const sorted = sortEventsChronologically(events);
    expect(sorted.map((e) => e.id)).toEqual(["evt_1", "evt_2"]);
  });

  it("computes overall status and root cause from failure evidence", () => {
    const overall = computeOverallStatus(change, events);
    expect(overall.status).toBe("failed");
    expect(overall.breakingEvent?.id).toBe("evt_2");

    const rootCause = inferRootCause(change, events);
    expect(rootCause.text.toLowerCase()).toContain("root cause");
    expect(rootCause.text).toContain("abc123");
    expect(rootCause.confidence).toBe("high");
  });

  it("computes time to detect and extracts correlators safely", () => {
    const ttd = computeTimeToDetect(change, events);
    expect(ttd.label).toBe("3m 0s");

    const correlators = extractCorrelators(change);
    expect(correlators.revision).toBe("abc123");
    expect(correlators.externalChangeId).toBe("CHG-1");
    expect(correlators.ticketId).toBe("JIRA-7");
    expect(correlators.approvalRef).toBe("APR-9");

    const unknown = inferRootCause(null, []);
    expect(unknown.text).toBe("Root cause: unknown (insufficient evidence)");
  });
});
