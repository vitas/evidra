import { useEffect, useMemo, useState, type KeyboardEvent } from "react";
import { CollapsibleRawEvidence } from "./components/CollapsibleRawEvidence";
import { EvidenceSummary } from "./components/EvidenceSummary";
import { IncidentTimeline } from "./components/IncidentTimeline";
import { RootCauseBanner } from "./components/RootCauseBanner";
import { SubjectSelect } from "./components/SubjectSelect";
import type { ChangeDetail, ChangeEvidence, ChangeItem, EventItem, ExportJob, SubjectInfo } from "./types";
import { buildEvidenceSummary } from "./ux";
import { computeOverallStatus, sortEventsChronologically } from "./utils/evidenceSummary";

type RuntimeCfg = {
  apiBase?: string;
  authMode?: string;
  authToken?: string;
};

const runtimeCfg = resolveRuntimeConfig();
const apiBase = (runtimeCfg.apiBase || "").replace(/\/$/, "");
const authMode = runtimeCfg.authMode || "none";
const authToken = runtimeCfg.authToken || "";
const changePathPrefix = "/ui/explorer/change/";
const embeddedMode = new URL(window.location.href).searchParams.get("embedded") === "argocd";
const buildLabel = String(import.meta.env.VITE_UI_BUILD || "dev");

type Status = "idle" | "error";
type ChangesState = "idle" | "loading" | "loaded" | "empty" | "error";

export function App() {
  const initialURLState = parseInitialURLState();
  const [subjects, setSubjects] = useState<string[]>([]);
  const [subject, setSubject] = useState(initialURLState.subject);
  const [from, setFrom] = useState(initialURLState.from);
  const [to, setTo] = useState(initialURLState.to);
  const [resultStatus, setResultStatus] = useState(initialURLState.resultStatus);
  const [externalChangeID, setExternalChangeID] = useState(initialURLState.externalChangeID);
  const [ticketID, setTicketID] = useState(initialURLState.ticketID);
  const [approvalReference, setApprovalReference] = useState(initialURLState.approvalReference);
  const [corrKey, setCorrKey] = useState(initialURLState.corrKey);
  const [q, setQ] = useState(initialURLState.q);
  const [statusText, setStatusText] = useState("Ready.");
  const [statusKind, setStatusKind] = useState<Status>("idle");
  const [changesState, setChangesState] = useState<ChangesState>("idle");
  const [changes, setChanges] = useState<ChangeItem[]>([]);
  const [selectedChange, setSelectedChange] = useState<ChangeDetail | null>(null);
  const [selectedEvents, setSelectedEvents] = useState<EventItem[]>([]);
  const [supportingCount, setSupportingCount] = useState(0);
  const [approvalsCount, setApprovalsCount] = useState(0);
  const [selectedEventID, setSelectedEventID] = useState("");
  const [nextCursor, setNextCursor] = useState("");
  const [lastExport, setLastExport] = useState<ExportJob | null>(null);
  const [autoSearchEnabled, setAutoSearchEnabled] = useState(false);
  const [detailAuthError, setDetailAuthError] = useState(false);

  const sortedEvents = useMemo(() => sortEventsChronologically(selectedEvents), [selectedEvents]);
  const selectedEvent = useMemo(() => sortedEvents.find((e) => e.id === selectedEventID) || sortedEvents[0] || null, [sortedEvents, selectedEventID]);
  const overallStatus = useMemo(() => computeOverallStatus(selectedChange, sortedEvents), [selectedChange, sortedEvents]);
  const evidenceGroups = useMemo(() => buildEvidenceSummary(selectedChange, sortedEvents), [selectedChange, sortedEvents]);
  const activeFilterCount = useMemo(
    () => [resultStatus, externalChangeID, ticketID, approvalReference, corrKey, q]
      .filter((value) => value.trim() !== "").length,
    [resultStatus, externalChangeID, ticketID, approvalReference, corrKey, q],
  );
  const filtersHelperText = useMemo(() => {
    if (statusKind === "error") return statusText;
    if (activeFilterCount > 0 && changes.length > 0) return `${changes.length} changes match current filters`;
    return "";
  }, [statusKind, statusText, activeFilterCount, changes.length]);

  useEffect(() => {
    void loadSubjects();
  }, []);

  useEffect(() => {
    if (!autoSearchEnabled) return;
    const timer = window.setTimeout(() => {
      void loadChanges("", true);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [subject, from, to, resultStatus, q, externalChangeID, ticketID, approvalReference, autoSearchEnabled]);

  useEffect(() => {
    const id = changeIDFromPath(window.location.pathname);
    if (id && subject) {
      void loadChanges(id, true);
    }
  }, [subject]);

  async function loadSubjects() {
    try {
      const data = await apiGet<{ items: SubjectInfo[] }>("/v1/subjects");
      const list = Array.from(new Set((data.items || []).map((s) => `${s.subject}:${s.namespace}:${s.cluster}`)));
      setSubjects(list);
      if (list.length > 0) {
        setSubject((prev) => prev || list[0]);
      }
      setAutoSearchEnabled(true);
      setStatus("");
    } catch (err) {
      setStatus(`Failed to load subjects: ${(err as Error).message}`, true);
    }
  }

  async function loadChanges(preferredID = "", reset = true) {
    if (!subject) {
      setStatus("Select a subject before loading changes.", true);
      return;
    }
    const fromRFC = localToRFC3339(from);
    const toRFC = localToRFC3339(to);
    if (!fromRFC || !toRFC) {
      setStatus("Set valid from/to timestamps.", true);
      return;
    }
    try {
      if (reset) setChangesState("loading");
      const params = new URLSearchParams({ subject, from: fromRFC, to: toRFC, limit: "50" });
      if (resultStatus) params.set("result_status", resultStatus);
      if (externalChangeID) params.set("external_change_id", externalChangeID);
      if (ticketID) params.set("ticket_id", ticketID);
      if (approvalReference) params.set("approval_reference", approvalReference);
      if (q) params.set("q", q);
      if (!reset && nextCursor) params.set("cursor", nextCursor);

      const resp = await apiGet<{ items: ChangeItem[]; page?: { next_cursor?: string } }>(`/v1/changes?${params.toString()}`);
      const items = resp.items || [];
      const mergedCount = (reset ? 0 : changes.length) + items.length;
      setNextCursor(resp.page?.next_cursor || "");
      setChanges((prev) => (reset ? items : [...prev, ...items]));

      if (mergedCount === 0) {
        setSelectedChange(null);
        setSelectedEvents([]);
        setSelectedEventID("");
        setSupportingCount(0);
        setApprovalsCount(0);
        setDetailAuthError(false);
        syncURL("");
        setChangesState("empty");
        setStatus("");
        return;
      }

      setChangesState("loaded");
      if (reset) await loadChangeDetail(preferredID || items[0].id);
      setStatus("");
    } catch (err) {
      if (reset) setChangesState("error");
      setStatus(`Failed to load changes: ${(err as Error).message}`, true);
    }
  }

  async function loadChangeDetail(id: string) {
    const fromRFC = localToRFC3339(from);
    const toRFC = localToRFC3339(to);
    if (!subject || !fromRFC || !toRFC) return;

    try {
      const params = new URLSearchParams({ subject, from: fromRFC, to: toRFC });
      const detail = await apiGet<ChangeDetail>(`/v1/changes/${encodeURIComponent(id)}?${params.toString()}`);
      const timeline = await apiGet<{ items: EventItem[] }>(`/v1/changes/${encodeURIComponent(id)}/timeline?${params.toString()}`);
      const evidence = await apiGet<ChangeEvidence>(`/v1/changes/${encodeURIComponent(id)}/evidence?${params.toString()}`);

      const sorted = sortEventsChronologically(timeline.items || []);
      setSelectedChange(detail);
      setSelectedEvents(sorted);
      setSupportingCount(evidence.supporting_observations?.length || 0);
      setApprovalsCount(evidence.approvals?.length || 0);
      setSelectedEventID(sorted[0]?.id || "");
      setDetailAuthError(false);
      syncURL(detail.id);
    } catch (err) {
      setDetailAuthError(isUnauthorized(err));
      setStatus(`Failed to load change detail: ${(err as Error).message}`, true);
    }
  }

  async function searchCorrelationFallback() {
    const value = q.trim();
    if (!value) {
      setStatus("Set correlation value.", true);
      return;
    }
    if (!corrKey) {
      await loadChanges("", true);
      setStatus("Applied text search with current filters.");
      return;
    }
    try {
      const params = new URLSearchParams({ value });
      const resp = await apiGet<{ items: EventItem[] }>(`/v1/correlations/${encodeURIComponent(corrKey)}?${params.toString()}`);
      const events = resp.items || [];
      if (events.length === 0) {
        setStatus("No correlation matches.");
        return;
      }
      setStatus(`Correlation fallback returned ${events.length} events. Narrow with subject/time and load changes.`);
    } catch (err) {
      setStatus(`Correlation search failed: ${(err as Error).message}`, true);
    }
  }

  async function createExport() {
    const fromRFC = localToRFC3339(from);
    const toRFC = localToRFC3339(to);
    if (!subject || !fromRFC || !toRFC) {
      setStatus("Subject and time range are required for export.", true);
      return;
    }
    const filter: Record<string, string> = { subject, from: fromRFC, to: toRFC };
    if (q.trim() && corrKey) {
      filter.correlation_key = corrKey;
      filter.correlation_value = q.trim();
    }
    try {
      const job = await apiPost<ExportJob>("/v1/exports", { format: "json", filter });
      setLastExport(job);
      setStatus(`Export created: ${job.id} (${job.status}). Polling...`);
      await pollExport(job.id);
    } catch (err) {
      setStatus(`Export failed: ${(err as Error).message}`, true);
    }
  }

  async function pollExport(id: string) {
    for (let i = 0; i < 20; i += 1) {
      await sleep(1000);
      const job = await apiGet<ExportJob>(`/v1/exports/${encodeURIComponent(id)}`);
      setLastExport(job);
      if (job.status === "completed") {
      setStatus(`Export completed: ${job.id}`);
        return;
      }
      if (job.status === "failed") {
        setStatus(`Export failed: ${job.error || "unknown error"}`, true);
        return;
      }
    }
      setStatus(`Export polling timed out for ${id}.`, true);
  }

  function downloadLastExport() {
    if (!lastExport?.id) return;
    window.open(`${apiBase}/v1/exports/${encodeURIComponent(lastExport.id)}/download`, "_blank", "noopener,noreferrer");
  }

  function setStatus(msg: string, error = false) {
    setStatusText(msg);
    setStatusKind(error ? "error" : "idle");
  }

  function syncURL(changeID: string) {
    const url = new URL(window.location.href);
    url.pathname = changeID ? `${changePathPrefix}${encodeURIComponent(changeID)}` : "/ui/";
    url.search = "";
    const fromRFC = localToRFC3339(from);
    const toRFC = localToRFC3339(to);
    if (subject) url.searchParams.set("subject", subject);
    if (fromRFC) url.searchParams.set("from", fromRFC);
    if (toRFC) url.searchParams.set("to", toRFC);
    if (resultStatus) url.searchParams.set("result_status", resultStatus);
    if (externalChangeID) url.searchParams.set("external_change_id", externalChangeID);
    if (ticketID) url.searchParams.set("ticket_id", ticketID);
    if (approvalReference) url.searchParams.set("approval_reference", approvalReference);
    if (q) url.searchParams.set("q", q);
    if (corrKey) url.searchParams.set("corr_key", corrKey);
    window.history.pushState({}, "", url.toString());
  }

  function applyQuickFilter(kind: "external" | "ticket" | "approval", value: string) {
    const next = value.trim();
    if (!next) return;
    if (kind === "external") setExternalChangeID(next);
    if (kind === "ticket") setTicketID(next);
    if (kind === "approval") setApprovalReference(next);
    window.setTimeout(() => void loadChanges("", true), 0);
  }

  function handleCorrelationEnter(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key !== "Enter") return;
    e.preventDefault();
    void loadChanges("", true);
  }

  function copyToClipboard(value: string, label: string) {
    const text = value.trim();
    if (!text) return;
    void navigator.clipboard.writeText(text)
      .then(() => setStatus(`Copied ${label}.`))
      .catch(() => setStatus(`Unable to copy ${label}.`, true));
  }

  function copyRevision() {
    const revision = (selectedChange?.revision || "").trim();
    if (!revision) return;
    copyToClipboard(revision, "revision");
  }

  function currentPermalink(changeID: string): string {
    return absoluteChangeLink(changeID, {
      subject, from, to, resultStatus, externalChangeID, ticketID, approvalReference, q, corrKey,
    });
  }

  const payloadSource = selectedEvent || selectedChange || {};

  return (
    <main className={`layout ${embeddedMode ? "layout-embedded" : ""}`.trim()} data-testid="explorer-root">
      <header className="topbar">
        <h1>Evidence Explorer</h1>
        <p>Investigation view for correlated GitOps evidence.</p>
      </header>

      <section className="controls" aria-label="Filters" data-testid="filters-panel">
        <div className="row">
          <label>Subject
            <SubjectSelect value={subject} onChange={setSubject} options={subjects} />
          </label>
          <label>From
            <input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} data-testid="from-input" />
          </label>
          <label>To
            <input type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} data-testid="to-input" />
          </label>
          <label>Result status
            <select value={resultStatus} onChange={(e) => setResultStatus(e.target.value)} data-testid="result-status-select">
              <option value="">Any</option>
              <option value="succeeded">succeeded</option>
              <option value="failed">failed</option>
              <option value="unknown">unknown</option>
            </select>
          </label>
          <label>External Change ID
            <input type="text" value={externalChangeID} onChange={(e) => setExternalChangeID(e.target.value)} data-testid="external-change-id-input" placeholder="CHG123456" />
          </label>
          <label>Ticket ID
            <input type="text" value={ticketID} onChange={(e) => setTicketID(e.target.value)} data-testid="ticket-id-input" placeholder="JIRA-42" />
          </label>
          <button type="button" onClick={() => void loadChanges("", true)} data-testid="find-changes-button">Find changes</button>
        </div>

        <details className="advanced-block" data-testid="advanced-correlation">
          <summary>Advanced correlation</summary>
          <div className="row">
            <label>Approval reference
              <input type="text" value={approvalReference} onChange={(e) => setApprovalReference(e.target.value)} data-testid="approval-reference-input" placeholder="APR-7" />
            </label>
            <label>Correlation key
              <select value={corrKey} onChange={(e) => setCorrKey(e.target.value)} data-testid="correlation-key-select">
                <option value="">any key</option>
                <option value="repo">repo</option>
                <option value="commit_sha">commit_sha</option>
                <option value="pr_id">pr_id</option>
                <option value="argocd_app">argocd_app</option>
                <option value="sync_revision">sync_revision</option>
                <option value="deploy_id">deploy_id</option>
                <option value="operation_id">operation_id</option>
                <option value="ticket_key">ticket_key</option>
                <option value="ticket_id">ticket_id</option>
                <option value="external_change_id">external_change_id</option>
                <option value="approval_reference">approval_reference</option>
              </select>
            </label>
            <label className="grow">Correlation value
              <input type="text" value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={handleCorrelationEnter} data-testid="correlation-value-input" placeholder="abc123 / PROJ-42 / 12345" />
            </label>
            <button type="button" onClick={() => void searchCorrelationFallback()} data-testid="search-raw-events-button">Search raw events</button>
            <button type="button" onClick={() => void createExport()} data-testid="create-export-button">Create export</button>
            {lastExport?.status === "completed" ? (
              <button type="button" onClick={downloadLastExport} data-testid="download-export-button">Download export</button>
            ) : null}
            {lastExport ? <span className="meta" data-testid="export-status">Export {lastExport.id}: {lastExport.status}</span> : null}
          </div>
        </details>
      </section>

      {filtersHelperText ? (
        <p className={`filters-helper ${statusKind === "error" ? "error" : ""}`} aria-live="polite" data-testid="filters-helper">
          {filtersHelperText}
        </p>
      ) : null}

      <section className="content" data-testid="content-layout">
        <aside className="panel" aria-label="Changes" data-testid="changes-panel">
          <h2>Changes ({changes.length})</h2>
          {changes.length === 0 ? (
            <p className="meta empty-hint" data-testid="changes-empty-state" data-state={changesState}>
              {changesState === "idle" ? "Adjust filters or search by correlation key to find production changes." : null}
              {changesState === "loading" ? "Searching changes..." : null}
              {changesState === "empty" ? "No changes found. Adjust filters or search by correlation key to find production changes." : null}
              {changesState === "error" ? "Unable to load changes. Adjust filters and retry." : null}
            </p>
          ) : null}
          <ul id="changeList" data-testid="changes-list">
            {changes.map((c) => (
              <li key={c.id} className={`item ${selectedChange?.id === c.id ? "selected" : ""}`.trim()} onClick={() => void loadChangeDetail(c.id)} data-testid="change-item" data-change-id={c.id} aria-selected={selectedChange?.id === c.id}>
                <strong>{c.application || c.subject}</strong>
                <span className={`badge ${c.result_status}`}>{c.result_status}</span>
                <div className="meta">{c.id} | {fmtDate(c.started_at)} - {fmtDate(c.completed_at)}</div>
                <div className="meta chips">
                  <span className="chip">Ext: {c.external_change_id || "n/a"}</span>
                  <span className="chip">Ticket: {c.ticket_id || "n/a"}</span>
                  <span className="chip">Approval: {c.approval_reference || "n/a"}</span>
                </div>
              </li>
            ))}
          </ul>
          {nextCursor ? <button type="button" onClick={() => void loadChanges("", false)} data-testid="load-more-button">Load more</button> : null}
        </aside>

        <section className="panel main" aria-label="Timeline and details" data-testid="timeline-panel">
          {selectedChange ? (
            <RootCauseBanner
              change={selectedChange}
              events={sortedEvents}
              permalink={currentPermalink(selectedChange.change_id || selectedChange.id)}
              onCopyPermalink={() => copyToClipboard(currentPermalink(selectedChange.change_id || selectedChange.id), "permalink")}
              onExport={() => void createExport()}
            />
          ) : (
            <div className="selected-change-bar" data-testid="selected-change-header"><div className="meta">No change selected.</div></div>
          )}

          {selectedChange ? (
            <section className="selected-change-bar selected-change-header" data-testid="selected-change-header">
              <div className="selected-change-header__primary">
                <div className="selected-change-header__id">
                  <strong>{selectedChange.change_id || selectedChange.id}</strong>
                  <span className={`badge ${overallStatus.status}`}>{overallStatus.status}</span>
                </div>
                <div><strong>Application:</strong> {selectedChange.application || selectedChange.subject}</div>
                <div>
                  <strong>Revision:</strong> {selectedChange.revision || "n/a"}
                  {selectedChange.revision ? (
                    <button type="button" className="secondary inline-button" onClick={copyRevision} data-testid="copy-revision-button">Copy revision</button>
                  ) : null}
                </div>
                <div className="chips">
                  {selectedChange.external_change_id ? <span className="chip">Change {selectedChange.external_change_id}</span> : null}
                  {selectedChange.ticket_id ? <span className="chip">Ticket {selectedChange.ticket_id}</span> : null}
                </div>
                <button type="button" className="secondary" onClick={() => void createExport()} data-testid="export-header-button">Export</button>
              </div>

              <details className="selected-change-header__secondary">
                <summary>More details</summary>
                <div className="meta">
                  Project: {selectedChange.project || "n/a"} | Cluster: {selectedChange.target_cluster || "n/a"} | Namespace: {selectedChange.namespace || "n/a"}
                </div>
                <div className="meta">
                  Initiator: {selectedChange.initiator || "n/a"} | Started: {fmtDate(selectedChange.started_at)} | Completed: {fmtDate(selectedChange.completed_at)}
                </div>
                <div className="meta">
                  Supporting observations: {supportingCount}
                </div>
              </details>
            </section>
          ) : null}

          <IncidentTimeline
            events={sortedEvents}
            selectedEventID={selectedEvent?.id || ""}
            onSelect={setSelectedEventID}
            breakingEventID={overallStatus.breakingEvent?.id || ""}
          />

          <EvidenceSummary groups={evidenceGroups} />

          <section className="evidence-links meta">
            <strong>References:</strong> {selectedChange ? (
              <>
                External{" "}
                <button type="button" className="linkish" onClick={() => applyQuickFilter("external", selectedChange.external_change_id || "")}>
                  {selectedChange.external_change_id || "n/a"}
                </button>{" "}
                | Ticket{" "}
                <button type="button" className="linkish" onClick={() => applyQuickFilter("ticket", selectedChange.ticket_id || "")}>
                  {selectedChange.ticket_id || "n/a"}
                </button>{" "}
                | Approval{" "}
                <button type="button" className="linkish" onClick={() => applyQuickFilter("approval", selectedChange.approval_reference || "")}>
                  {selectedChange.approval_reference || "n/a"}
                </button>{" "}
                | Approvals: {approvalsCount}
              </>
            ) : "n/a"}
          </section>

          <CollapsibleRawEvidence payload={payloadSource} selected={!!selectedChange || !!selectedEvent} detailAuthError={detailAuthError} />
        </section>
      </section>

      <footer className="build-info" data-testid="ui-build-info">
        UI build: {buildLabel} ({embeddedMode ? "argocd-embedded" : "standalone"})
      </footer>
    </main>
  );
}

type InitialURLState = {
  subject: string;
  from: string;
  to: string;
  resultStatus: string;
  externalChangeID: string;
  ticketID: string;
  approvalReference: string;
  corrKey: string;
  q: string;
};

function parseInitialURLState(): InitialURLState {
  const url = new URL(window.location.href);
  return {
    subject: url.searchParams.get("subject") || "",
    from: rfc3339ToLocal(url.searchParams.get("from") || "") || defaultFrom(),
    to: rfc3339ToLocal(url.searchParams.get("to") || "") || defaultTo(),
    resultStatus: url.searchParams.get("result_status") || "",
    externalChangeID: url.searchParams.get("external_change_id") || "",
    ticketID: url.searchParams.get("ticket_id") || "",
    approvalReference: url.searchParams.get("approval_reference") || "",
    corrKey: url.searchParams.get("corr_key") || "",
    q: url.searchParams.get("q") || "",
  };
}

async function apiGet<T>(path: string): Promise<T> {
  const req: RequestInit = { headers: authHeaders() };
  if (authMode === "cookie") req.credentials = "include";
  const res = await fetch(`${apiBase}${path}`, req);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body?.error?.message || `HTTP ${res.status}`);
  return body as T;
}

async function apiPost<T>(path: string, payload: unknown): Promise<T> {
  const headers = authHeaders();
  headers["Content-Type"] = "application/json";
  const req: RequestInit = { method: "POST", headers, body: JSON.stringify(payload) };
  if (authMode === "cookie") req.credentials = "include";
  const res = await fetch(`${apiBase}${path}`, req);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body?.error?.message || `HTTP ${res.status}`);
  return body as T;
}

function defaultFrom(): string {
  const to = new Date();
  return toLocalInput(new Date(to.getTime() - 7 * 24 * 60 * 60 * 1000));
}

function defaultTo(): string {
  return toLocalInput(new Date());
}

function toLocalInput(d: Date): string {
  const adjusted = new Date(d.getTime() - d.getTimezoneOffset() * 60000);
  return adjusted.toISOString().slice(0, 16);
}

function localToRFC3339(v: string): string {
  if (!v) return "";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? "" : d.toISOString();
}

function rfc3339ToLocal(v: string): string {
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? "" : toLocalInput(d);
}

function fmtDate(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toISOString().replace("T", " ").replace("Z", " UTC");
}

type LinkContext = {
  subject?: string;
  from?: string;
  to?: string;
  resultStatus?: string;
  externalChangeID?: string;
  ticketID?: string;
  approvalReference?: string;
  q?: string;
  corrKey?: string;
};

function absoluteChangeLink(changeID: string, ctx?: LinkContext): string {
  const id = encodeURIComponent(changeID);
  const url = new URL(`${window.location.origin}${changePathPrefix}${id}`);
  const fromRFC = localToRFC3339(ctx?.from || "");
  const toRFC = localToRFC3339(ctx?.to || "");
  if (ctx?.subject) url.searchParams.set("subject", ctx.subject);
  if (fromRFC) url.searchParams.set("from", fromRFC);
  if (toRFC) url.searchParams.set("to", toRFC);
  if (ctx?.resultStatus) url.searchParams.set("result_status", ctx.resultStatus);
  if (ctx?.externalChangeID) url.searchParams.set("external_change_id", ctx.externalChangeID);
  if (ctx?.ticketID) url.searchParams.set("ticket_id", ctx.ticketID);
  if (ctx?.approvalReference) url.searchParams.set("approval_reference", ctx.approvalReference);
  if (ctx?.q) url.searchParams.set("q", ctx.q);
  if (ctx?.corrKey) url.searchParams.set("corr_key", ctx.corrKey);
  return url.toString();
}

function isUnauthorized(err: unknown): boolean {
  const msg = String((err as Error)?.message || "");
  return msg.includes("HTTP 401") || msg.includes("HTTP 403");
}

function changeIDFromPath(pathname: string): string {
  if (!pathname.startsWith(changePathPrefix)) return "";
  return decodeURIComponent(pathname.slice(changePathPrefix.length).split("/")[0] || "");
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function resolveRuntimeConfig(): RuntimeCfg {
  const winCfg = (window as unknown as { __EVIDRA_CONFIG__?: RuntimeCfg }).__EVIDRA_CONFIG__ || {};
  const url = new URL(window.location.href);
  return {
    apiBase: ((url.searchParams.get("api_base") || "").trim() || winCfg.apiBase || "").replace(/\/$/, ""),
    authMode: (url.searchParams.get("auth_mode") || "").trim() || winCfg.authMode || "none",
    authToken: (url.searchParams.get("auth_token") || "").trim() || winCfg.authToken || "",
  };
}

function authHeaders(): Record<string, string> {
  if (authMode === "bearer" && authToken) return { Authorization: `Bearer ${authToken}` };
  return {};
}
