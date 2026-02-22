import type { ChangeDetail, EventItem } from "../types";

export type InvestigationStatus = "failed" | "succeeded" | "unknown";

export type OverallStatus = {
  status: InvestigationStatus;
  breakingEvent?: EventItem;
};

export type RootCauseInference = {
  text: string;
  confidence: "high" | "medium" | "low";
  eventRef?: string;
};

export type FormattedRootCause = {
  primaryText: string;
  correlatorTokens: string[];
  timestampText?: string;
};

export type TimeToDetect = {
  seconds?: number;
  label: string;
};

export type Correlators = {
  revision?: string;
  externalChangeId?: string;
  ticketId?: string;
  approvalRef?: string;
};

export type EventCategory = "git" | "approval" | "argo" | "other";

export function sortEventsChronologically(events: EventItem[]): EventItem[] {
  return [...events].sort((a, b) => asTime(a.time) - asTime(b.time));
}

export function computeOverallStatus(change: ChangeDetail | null, events: EventItem[] = []): OverallStatus {
  const ordered = sortEventsChronologically(events);
  const breakingEvent = ordered.find(isFailureEvent);
  if (breakingEvent) return { status: "failed", breakingEvent };

  const status = String(change?.result_status || "unknown").toLowerCase();
  if (status === "failed") return { status: "failed" };
  if (status === "succeeded") return { status: "succeeded" };
  return { status: "unknown" };
}

export function inferRootCause(change: ChangeDetail | null, events: EventItem[] = []): RootCauseInference {
  const overall = computeOverallStatus(change, events);
  const correlators = extractCorrelators(change);
  if (!overall.breakingEvent) {
    return {
      text: "Root cause: unknown (insufficient evidence)",
      confidence: "low",
    };
  }

  const event = overall.breakingEvent;
  const parts: string[] = [];
  if (correlators.revision) parts.push(`revision ${correlators.revision}`);
  if (correlators.externalChangeId) parts.push(`change ${correlators.externalChangeId}`);
  if (correlators.ticketId) parts.push(`ticket ${correlators.ticketId}`);

  const context = parts.length > 0 ? ` linked to ${parts.join(", ")}` : "";
  return {
    text: `Root cause: ${humanizeEventType(event.type)} observed at ${formatTimestamp(event.time)}${context}.`,
    confidence: "high",
    eventRef: event.id,
  };
}

export function formatRootCause(change: ChangeDetail | null, events: EventItem[] = []): FormattedRootCause {
  const overall = computeOverallStatus(change, events);
  const correlators = extractCorrelators(change);
  const shortRevision = shortenRevision(correlators.revision);
  const tokens = [
    correlators.externalChangeId,
    correlators.ticketId,
    correlators.revision,
  ].filter((v): v is string => typeof v === "string" && v.trim() !== "");

  if (!overall.breakingEvent) {
    return {
      primaryText: "Root cause: unknown (insufficient evidence)",
      correlatorTokens: tokens,
    };
  }

  const eventLabel = humanizeEventType(overall.breakingEvent.type);
  const primary = shortRevision
    ? `${eventLabel} after revision ${shortRevision}`
    : `${eventLabel} after deployment`;

  return {
    primaryText: truncate(primary, 90),
    correlatorTokens: tokens,
    timestampText: formatTimestamp(overall.breakingEvent.time),
  };
}

export function computeTimeToDetect(change: ChangeDetail | null, events: EventItem[] = []): TimeToDetect {
  if (!change) return { label: "n/a" };

  const ordered = sortEventsChronologically(events);
  const failure = ordered.find(isFailureEvent);
  if (!failure) return { label: "n/a" };

  const start = asTime(change.started_at || change.completed_at);
  const end = asTime(failure.time);
  if (start <= 0 || end <= 0 || end < start) return { label: "n/a" };

  const seconds = Math.floor((end - start) / 1000);
  if (seconds < 60) return { seconds, label: `${seconds}s` };

  const min = Math.floor(seconds / 60);
  const sec = seconds % 60;
  return { seconds, label: `${min}m ${sec}s` };
}

export function extractCorrelators(change: ChangeDetail | null): Correlators {
  if (!change) return {};
  return {
    revision: emptyToUndefined(change.revision),
    externalChangeId: emptyToUndefined(change.external_change_id),
    ticketId: emptyToUndefined(change.ticket_id),
    approvalRef: emptyToUndefined(change.approval_reference),
  };
}

export function categorizeEvent(event: EventItem): EventCategory {
  const text = `${event.source} ${event.type}`.toLowerCase();
  if (text.includes("git") || text.includes("commit") || text.includes("revision")) return "git";
  if (text.includes("approval") || text.includes("ticket") || text.includes("pr")) return "approval";
  if (text.includes("argo") || text.includes("sync") || text.includes("health")) return "argo";
  return "other";
}

export function humanizeEventType(eventType: string): string {
  const raw = (eventType || "event").replace(/[._]/g, " ").trim();
  if (!raw) return "event";
  return raw
    .split(/\s+/)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

export function extractEventStatus(event: EventItem): string {
  const md = event.extensions || {};
  const candidates = [md.status, md.phase, md.health, md.result]
    .filter((v): v is string => typeof v === "string" && v.trim() !== "")
    .map((v) => v.trim());
  if (candidates.length > 0) return candidates[0];
  return "";
}

export function isFailureEvent(event: EventItem): boolean {
  const signal = `${event.type} ${extractEventStatus(event)} ${JSON.stringify(event.extensions || {})}`.toLowerCase();
  return signal.includes("fail") || signal.includes("error") || signal.includes("degrad") || signal.includes("abort");
}

function asTime(value: string): number {
  const t = new Date(value).getTime();
  return Number.isFinite(t) ? t : 0;
}

function formatTimestamp(value: string): string {
  const t = asTime(value);
  if (!t) return "unknown time";
  return new Date(t).toISOString().replace("T", " ").replace("Z", " UTC");
}

function emptyToUndefined(value?: string): string | undefined {
  if (!value) return undefined;
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function truncate(text: string, limit: number): string {
  if (text.length <= limit) return text;
  return `${text.slice(0, Math.max(0, limit - 1)).trimEnd()}…`;
}

function shortenRevision(value?: string): string | undefined {
  const normalized = emptyToUndefined(value);
  if (!normalized) return undefined;
  return normalized.length > 12 ? normalized.slice(0, 12) : normalized;
}
