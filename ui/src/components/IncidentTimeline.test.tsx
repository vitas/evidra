import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { EventItem } from "../types";
import { IncidentTimeline } from "./IncidentTimeline";

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

describe("IncidentTimeline", () => {
  it("renders chronological events with category and break marker", () => {
    const onSelect = vi.fn();
    render(<IncidentTimeline events={events} selectedEventID="evt_1" onSelect={onSelect} breakingEventID="evt_2" />);

    const items = screen.getAllByTestId("timeline-item");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent("Argo Sync Finished");
    expect(items[1]).toHaveTextContent("Argo Health Changed");
    expect(items[1]).toHaveTextContent("breaking point");

    fireEvent.click(items[1]);
    expect(onSelect).toHaveBeenCalledWith("evt_2");
  });
});
