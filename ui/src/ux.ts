import type { ChangeDetail, EventItem } from "./types";

export type EvidenceSummaryGroup = {
  title: string;
  items: string[];
};

export function sortEventsChronologically(events: EventItem[]): EventItem[] {
  return [...events].sort((a, b) => safeTime(a.time) - safeTime(b.time));
}

export function eventSentence(event: EventItem): string {
  const ty = event.type || "event";
  const app = event.subject || "application";
  const status = extensionString(event, "status");
  const health = extensionString(event, "health");
  if (status) return `${ty} for ${app} with status ${status}.`;
  if (health) return `${ty} for ${app} with health ${health}.`;
  return `${ty} recorded for ${app}.`;
}

export function buildEvidenceSummary(change: ChangeDetail | null, events: EventItem[]): EvidenceSummaryGroup[] {
  if (!change) return [];
  const sorted = sortEventsChronologically(events);
  const first = sorted[0];
  const last = sorted[sorted.length - 1];
  const healthAfter = change.health_after_deploy || change.health_status || "unknown";

  return [
    {
      title: "Incident Narrative",
      items: [
        `Deployment ${change.result_status} for ${change.application || change.subject}.`,
        `Change window: ${fmt(change.started_at)} to ${fmt(change.completed_at)}.`,
        `Health after deployment: ${String(healthAfter)}.`,
        first ? `First observed event: ${eventSentence(first)}` : "No timeline events recorded.",
        last ? `Latest observed event: ${eventSentence(last)}` : "No latest event available.",
      ],
    },
    {
      title: "Technical Context",
      items: [
        `Cluster: ${change.target_cluster || "n/a"}.`,
        `Namespace: ${change.namespace || "n/a"}.`,
        `Initiator: ${change.initiator || "n/a"}.`,
        `Primary provider: ${change.primary_provider}.`,
        `Revision: ${change.revision || "n/a"}.`,
      ],
    },
    {
      title: "Correlations",
      items: [
        `External change: ${change.external_change_id || "n/a"}.`,
        `Ticket: ${change.ticket_id || "n/a"}.`,
        `Approval reference: ${change.approval_reference || "n/a"}.`,
      ],
    },
  ];
}

function extensionString(event: EventItem, key: string): string {
  const md = event.extensions || {};
  const value = md[key];
  return typeof value === "string" ? value : "";
}

function safeTime(v: string): number {
  const t = new Date(v).getTime();
  return Number.isFinite(t) ? t : 0;
}

function fmt(v: string): string {
  const t = safeTime(v);
  if (!t) return "n/a";
  return new Date(t).toISOString().replace("T", " ").replace("Z", " UTC");
}
