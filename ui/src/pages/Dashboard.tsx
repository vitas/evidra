import { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router";
import { useAuth } from "../context/AuthContext";
import { useApi } from "../hooks/useApi";

type Period = "7d" | "30d" | "90d";

interface ScorecardData {
  score: number;
  band: string;
  basis: string;
  confidence: string;
  total_entries: number;
  signal_summary: Record<string, SignalDetail>;
}

interface SignalDetail {
  detected: boolean;
  count?: number;
  penalty?: number;
}

interface EvidenceEntry {
  id: string;
  type: string;
  tool: string;
  operation: string;
  scope: string;
  risk_level: string;
  actor: string;
  verdict?: string;
  exit_code?: number;
  created_at: string;
}

interface EntriesResponse {
  entries: EvidenceEntry[];
  total: number;
}

interface BreakdownItem {
  name: string;
  count: number;
  percent: number;
}

const SIGNAL_META: Record<string, { icon: string; label: string }> = {
  protocol_violation: { icon: "\u26A0", label: "Protocol Violation" },
  artifact_drift: { icon: "\u21C5", label: "Artifact Drift" },
  retry_loop: { icon: "\u21BA", label: "Retry Loop" },
  thrashing: { icon: "\u21AF", label: "Thrashing" },
  blast_radius: { icon: "\u25C9", label: "Blast Radius" },
  risk_escalation: { icon: "\u2191", label: "Risk Escalation" },
  new_scope: { icon: "\u2737", label: "New Scope" },
  repair_loop: { icon: "\u2795", label: "Repair Loop" },
};

function computeBreakdown(entries: EvidenceEntry[], field: keyof EvidenceEntry): BreakdownItem[] {
  const counts: Record<string, number> = {};
  for (const e of entries) {
    const val = (e[field] as string) || "unknown";
    counts[val] = (counts[val] || 0) + 1;
  }
  const total = entries.length || 1;
  return Object.entries(counts)
    .map(([name, count]) => ({ name, count, percent: (count / total) * 100 }))
    .sort((a, b) => b.count - a.count);
}

function computeRiskBreakdown(entries: EvidenceEntry[]): BreakdownItem[] {
  const counts: Record<string, number> = { high: 0, medium: 0, low: 0 };
  for (const e of entries) {
    const level = e.risk_level || "low";
    counts[level] = (counts[level] || 0) + 1;
  }
  const total = entries.length || 1;
  return ["high", "medium", "low"]
    .filter((k) => counts[k] > 0)
    .map((name) => ({ name, count: counts[name], percent: (counts[name] / total) * 100 }));
}

export function Dashboard() {
  const { apiKey } = useAuth();

  if (!apiKey) {
    return <AuthGate />;
  }

  return <DashboardContent />;
}

function AuthGate() {
  const { setApiKey } = useAuth();
  const [input, setInput] = useState("");
  const navigate = useNavigate();

  const handleSubmit = () => {
    if (input.trim()) {
      setApiKey(input.trim());
    }
  };

  return (
    <section className="relative min-h-[calc(100vh-60px)] flex items-center justify-center bg-[radial-gradient(ellipse_60%_40%_at_50%_0%,var(--color-accent-subtle),var(--color-bg)_70%)]">
      <div className="absolute inset-0 bg-[radial-gradient(circle,var(--color-accent)_1px,transparent_1px)] bg-[length:24px_24px] opacity-[0.04] [mask-image:radial-gradient(ellipse_50%_50%_at_50%_20%,black,transparent)]" />
      <div className="relative max-w-[440px] w-full mx-auto px-6">
        <div className="bg-bg-elevated border border-border rounded-xl p-8 shadow-[var(--shadow-card-lg)]">
          <div className="text-center mb-6">
            <div className="w-12 h-12 mx-auto mb-4 rounded-xl bg-accent-subtle border border-border flex items-center justify-center text-xl">
              {"\uD83D\uDD12"}
            </div>
            <h2 className="text-[1.15rem] font-bold text-fg mb-2">Dashboard Access</h2>
            <p className="text-[0.85rem] text-fg-muted">
              Enter your API key to view reliability data.
            </p>
          </div>
          <input
            type="password"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
            placeholder="ev1_..."
            autoFocus
            className="w-full px-4 py-3 bg-[var(--color-code-bg)] border border-border rounded-lg font-mono text-[0.88rem] text-fg placeholder:text-fg-muted/40 outline-none transition-all focus:border-accent focus:shadow-[0_0_0_3px_rgba(5,150,105,0.12)] mb-4"
          />
          <button
            onClick={handleSubmit}
            disabled={!input.trim()}
            className="w-full px-5 py-3 rounded-lg text-[0.88rem] font-semibold bg-accent text-white transition-all hover:bg-accent-bright hover:-translate-y-px hover:shadow-lg cursor-pointer border-none disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Unlock Dashboard
          </button>
          <div className="mt-4 text-center">
            <button
              onClick={() => navigate("/onboarding")}
              className="text-[0.82rem] text-accent font-medium bg-transparent border-none cursor-pointer hover:underline"
            >
              Need a key? Get one here
            </button>
          </div>
        </div>
      </div>
    </section>
  );
}

const PAGE_SIZE = 20;

function DashboardContent() {
  const { apiKey, clearApiKey } = useAuth();
  const { request } = useApi();
  const [period, setPeriod] = useState<Period>("30d");
  const [scorecard, setScorecard] = useState<ScorecardData | null>(null);
  const [entries, setEntries] = useState<EvidenceEntry[]>([]);
  const [totalEntries, setTotalEntries] = useState(0);
  const [loading, setLoading] = useState(true);
  const [entriesLoading, setEntriesLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(0);

  const fetchScorecard = useCallback(async () => {
    return request<ScorecardData>(`/v1/evidence/scorecard?period=${period}`).catch(() => null);
  }, [request, period]);

  const fetchEntries = useCallback(async (offset: number) => {
    return request<EntriesResponse>(`/v1/evidence/entries?limit=${PAGE_SIZE}&offset=${offset}&period=${period}`).catch(() => null);
  }, [request, period]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    setPage(0);
    try {
      const [sc, ent] = await Promise.all([
        fetchScorecard(),
        fetchEntries(0),
      ]);
      if (sc) setScorecard(sc);
      if (ent) {
        setEntries(ent.entries || []);
        setTotalEntries(ent.total || 0);
      }
      if (!sc && !ent) {
        setError("Could not fetch data. Check your API key and server status.");
      }
    } catch {
      setError("Network error. Is the API server running?");
    } finally {
      setLoading(false);
    }
  }, [fetchScorecard, fetchEntries]);

  const handlePageChange = useCallback(async (newPage: number) => {
    setEntriesLoading(true);
    const ent = await fetchEntries(newPage * PAGE_SIZE);
    if (ent) {
      setEntries(ent.entries || []);
      setTotalEntries(ent.total || 0);
      setPage(newPage);
    }
    setEntriesLoading(false);
  }, [fetchEntries]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const totalPages = Math.ceil(totalEntries / PAGE_SIZE);

  const activeSignals = scorecard?.signal_summary
    ? Object.values(scorecard.signal_summary).filter((s) => s.detected).length
    : 0;
  const totalSignals = scorecard?.signal_summary
    ? Object.keys(scorecard.signal_summary).length
    : 8;

  const actorBreakdown = useMemo(() => computeBreakdown(entries, "actor"), [entries]);
  const toolBreakdown = useMemo(() => computeBreakdown(entries, "tool"), [entries]);
  const scopeBreakdown = useMemo(() => computeBreakdown(entries, "scope"), [entries]);
  const riskBreakdown = useMemo(() => computeRiskBreakdown(entries), [entries]);

  return (
    <section className="min-h-[calc(100vh-60px)] py-8 bg-bg">
      <div className="max-w-[980px] mx-auto px-8">
        {/* Header bar */}
        <div className="flex items-center justify-between mb-8 flex-wrap gap-4">
          <div>
            <h1 className="text-[1.35rem] font-extrabold text-fg tracking-tight">
              Reliability Dashboard
            </h1>
            <p className="text-[0.84rem] text-fg-muted mt-1">
              Behavioral signals and scoring from your evidence chain.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-1 font-mono text-[0.76rem] text-fg-muted bg-[var(--color-code-bg)] border border-border-subtle rounded-lg px-3 py-1.5">
              <span className="text-accent">{apiKey?.slice(0, 8)}</span>
              <span className="text-fg-muted/40">{"****"}</span>
            </div>
            <button
              onClick={clearApiKey}
              className="text-[0.78rem] font-medium text-fg-muted bg-transparent border border-border-subtle rounded-lg px-3 py-1.5 cursor-pointer transition-all hover:border-red-300 hover:text-red-500"
            >
              Disconnect
            </button>
          </div>
        </div>

        {/* Period filter */}
        <div className="flex items-center gap-2 mb-6">
          <span className="text-[0.78rem] font-medium text-fg-muted mr-1">Period:</span>
          {(["7d", "30d", "90d"] as Period[]).map((p) => (
            <button
              key={p}
              onClick={() => setPeriod(p)}
              className={`px-3 py-1.5 rounded-lg text-[0.78rem] font-mono font-medium border transition-all cursor-pointer ${
                period === p
                  ? "bg-accent/10 border-accent/30 text-accent"
                  : "bg-transparent border-border-subtle text-fg-muted hover:border-accent/20 hover:text-fg"
              }`}
            >
              {p}
            </button>
          ))}
          <button
            onClick={fetchData}
            disabled={loading}
            className="ml-auto text-[0.78rem] font-medium text-fg-muted bg-transparent border border-border-subtle rounded-lg px-3 py-1.5 cursor-pointer transition-all hover:border-accent hover:text-fg disabled:opacity-50"
          >
            {loading ? "Loading..." : "Refresh"}
          </button>
        </div>

        {error && (
          <div className="mb-6 px-4 py-3 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800/50 rounded-lg text-[0.84rem] text-red-700 dark:text-red-300">
            {error}
          </div>
        )}

        {/* Stat cards */}
        <div className="grid grid-cols-4 gap-4 mb-8 max-md:grid-cols-2 max-sm:grid-cols-1">
          <StatCard
            label="Score"
            value={scorecard ? (scorecard.score < 0 ? "\u2014" : String(Math.round(scorecard.score))) : "\u2014"}
            sub={scorecard?.confidence || ""}
            color={bandColor(scorecard?.band)}
            loading={loading}
          />
          <StatCard
            label="Band"
            value={scorecard ? formatBand(scorecard.band) : "\u2014"}
            sub={scorecard?.basis || ""}
            color={bandColor(scorecard?.band)}
            loading={loading}
          />
          <StatCard
            label="Signals"
            value={scorecard ? `${activeSignals} / ${totalSignals}` : "\u2014"}
            sub="active"
            color={activeSignals > 0 ? "amber" : "green"}
            loading={loading}
          />
          <StatCard
            label="Entries"
            value={totalEntries > 0 ? String(totalEntries) : "\u2014"}
            sub={period}
            color="default"
            loading={loading}
          />
        </div>

        {/* Breakdown panels: actors, tools, scopes, risk */}
        {entries.length > 0 && (
          <div className="grid grid-cols-4 gap-4 mb-8 max-md:grid-cols-2 max-sm:grid-cols-1">
            <BreakdownPanel title="Actors" items={actorBreakdown} icon={"\u{1F464}"} />
            <BreakdownPanel title="Tools" items={toolBreakdown} icon={"\u{1F527}"} />
            <BreakdownPanel title="Scopes" items={scopeBreakdown} icon={"\u{1F30D}"} />
            <BreakdownPanel title="Effective Risk" items={riskBreakdown} icon={"\u{26A1}"} colorFn={riskItemColor} />
          </div>
        )}

        {/* Signal breakdown */}
        {scorecard?.signal_summary && (
          <div className="mb-8">
            <SectionHeader title="Signal Breakdown" />
            <div className="grid grid-cols-1 gap-[1px] bg-border rounded-xl overflow-hidden shadow-[var(--shadow-card)]">
              {Object.entries(scorecard.signal_summary).map(([name, signal]) => {
                const meta = SIGNAL_META[name] || { icon: "\u25CB", label: name };
                return (
                  <div
                    key={name}
                    className={`bg-bg-elevated flex items-center gap-4 px-5 py-3.5 ${
                      signal.detected ? "" : "opacity-50"
                    }`}
                  >
                    <div className="w-7 h-7 rounded-lg bg-accent-subtle border border-border flex items-center justify-center text-sm shrink-0">
                      {meta.icon}
                    </div>
                    <div className="font-mono text-[0.8rem] font-semibold text-fg w-[160px] shrink-0">
                      {name}
                    </div>
                    <div className="flex-1 text-[0.76rem] text-fg-muted">
                      {typeof signal.count === "number" ? `${signal.count} event${signal.count === 1 ? "" : "s"}` : "No events"}
                    </div>
                    <div
                      className={`font-mono text-[0.74rem] w-[64px] text-right shrink-0 ${
                        signal.detected ? "text-accent font-medium" : "text-fg-muted/40"
                      }`}
                    >
                      {signal.detected ? "detected" : "clear"}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Evidence timeline */}
        <div className="mb-8">
          <SectionHeader title="Recent Evidence" />
          {entries.length === 0 && !loading ? (
            <div className="bg-bg-elevated border border-border rounded-xl p-10 text-center">
              <div className="text-3xl mb-3 opacity-40">{"\uD83D\uDCCB"}</div>
              <p className="text-[0.88rem] text-fg-muted mb-2">No evidence entries yet.</p>
              <p className="text-[0.82rem] text-fg-muted/60">
                Run <code className="text-[0.8rem]">evidra record</code> or connect an MCP agent to start recording.
              </p>
            </div>
          ) : (
            <div className={`bg-bg-elevated border border-border rounded-xl overflow-hidden shadow-[var(--shadow-card)] transition-opacity ${entriesLoading ? "opacity-50" : ""}`}>
              {/* Table header */}
              <div className="grid grid-cols-[80px_70px_1fr_1fr_1fr_70px] gap-3 px-5 py-2.5 bg-[var(--color-code-bg)] border-b border-border text-[0.72rem] font-mono font-medium text-fg-muted/60 uppercase tracking-wider max-md:hidden">
                <div>Time</div>
                <div>Type</div>
                <div>Actor</div>
                <div>Operation</div>
                <div>Scope</div>
                <div className="text-right">Effective Risk</div>
              </div>
              {entries.map((entry) => (
                <EntryRow key={entry.id} entry={entry} />
              ))}
            </div>
          )}
          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between mt-4 px-1">
              <div className="font-mono text-[0.74rem] text-fg-muted/60">
                {page * PAGE_SIZE + 1}&ndash;{Math.min((page + 1) * PAGE_SIZE, totalEntries)} of {totalEntries}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => handlePageChange(page - 1)}
                  disabled={page === 0 || entriesLoading}
                  className="px-3 py-1.5 rounded-lg text-[0.76rem] font-mono font-medium border border-border-subtle bg-transparent text-fg-muted cursor-pointer transition-all hover:border-accent hover:text-fg disabled:opacity-30 disabled:cursor-not-allowed"
                >
                  Prev
                </button>
                <span className="font-mono text-[0.74rem] text-fg-muted tabular-nums">
                  {page + 1} / {totalPages}
                </span>
                <button
                  onClick={() => handlePageChange(page + 1)}
                  disabled={page >= totalPages - 1 || entriesLoading}
                  className="px-3 py-1.5 rounded-lg text-[0.76rem] font-mono font-medium border border-border-subtle bg-transparent text-fg-muted cursor-pointer transition-all hover:border-accent hover:text-fg disabled:opacity-30 disabled:cursor-not-allowed"
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

/* ── Sub-components ── */

function StatCard({
  label,
  value,
  sub,
  color,
  loading,
}: {
  label: string;
  value: string;
  sub: string;
  color: string;
  loading: boolean;
}) {
  const borderClass =
    color === "green"
      ? "border-l-emerald-500"
      : color === "amber"
        ? "border-l-amber-500"
        : color === "red"
          ? "border-l-red-500"
          : "border-l-accent/30";

  return (
    <div
      className={`bg-bg-elevated border border-border border-l-[3px] ${borderClass} rounded-xl p-5 shadow-[var(--shadow-card)] transition-all ${
        loading ? "animate-pulse" : ""
      }`}
    >
      <div className="font-mono text-[0.68rem] font-medium text-fg-muted/60 uppercase tracking-wider mb-2">
        {label}
      </div>
      <div className="font-mono text-[1.6rem] font-bold text-fg tracking-tight leading-none mb-1 truncate" title={value}>
        {value}
      </div>
      {sub && (
        <div className="font-mono text-[0.72rem] text-fg-muted/50">{sub}</div>
      )}
    </div>
  );
}

function BreakdownPanel({
  title,
  items,
  icon,
  colorFn,
}: {
  title: string;
  items: BreakdownItem[];
  icon: string;
  colorFn?: (name: string) => string;
}) {
  const maxCount = items.length > 0 ? items[0].count : 1;

  return (
    <div className="bg-bg-elevated border border-border rounded-xl p-5 shadow-[var(--shadow-card)]">
      <div className="flex items-center gap-2 mb-4">
        <span className="text-sm">{icon}</span>
        <div className="font-mono text-[0.68rem] font-medium text-fg-muted/60 uppercase tracking-wider">
          {title}
        </div>
      </div>
      <div className="space-y-2.5">
        {items.slice(0, 5).map((item) => (
          <div key={item.name}>
            <div className="flex items-center justify-between mb-1">
              <span className="font-mono text-[0.76rem] text-fg truncate mr-2">
                {item.name}
              </span>
              <span className="font-mono text-[0.72rem] text-fg-muted shrink-0">
                {item.count}
              </span>
            </div>
            <div className="h-1 bg-bg-alt rounded-full overflow-hidden">
              <div
                className={`h-full rounded-full transition-all duration-500 ${
                  colorFn ? colorFn(item.name) : "bg-accent"
                }`}
                style={{ width: `${(item.count / maxCount) * 100}%` }}
              />
            </div>
          </div>
        ))}
        {items.length === 0 && (
          <div className="text-[0.76rem] text-fg-muted/40 text-center py-2">No data</div>
        )}
      </div>
    </div>
  );
}

function riskItemColor(name: string): string {
  if (name === "high") return "bg-red-500";
  if (name === "medium") return "bg-amber-500";
  return "bg-emerald-500";
}

function EntryRow({ entry }: { entry: EvidenceEntry }) {
  const time = formatTime(entry.created_at);
  const typeColor =
    entry.type === "prescription"
      ? "bg-emerald-500"
      : entry.type === "report"
        ? "bg-blue-500"
        : "bg-fg-muted";

  const riskColor =
    entry.risk_level === "high"
      ? "text-red-500"
      : entry.risk_level === "medium"
        ? "text-amber-500"
        : "text-fg-muted";

  return (
    <div className="grid grid-cols-[80px_70px_1fr_1fr_1fr_70px] gap-3 px-5 py-3 border-b border-border-subtle last:border-b-0 items-center hover:bg-bg-alt/30 transition-colors max-md:grid-cols-1 max-md:gap-1">
      <div className="font-mono text-[0.78rem] text-fg-muted tabular-nums">{time}</div>
      <div className="flex items-center gap-1.5">
        <span className={`w-1.5 h-1.5 rounded-full ${typeColor}`} />
        <span className="font-mono text-[0.74rem] text-fg">{entry.type === "prescription" ? "prescribe" : entry.type}</span>
      </div>
      <div className="font-mono text-[0.76rem] text-fg-muted truncate">{entry.actor || "\u2014"}</div>
      <div className="font-mono text-[0.78rem] text-fg-body truncate">
        {entry.tool && entry.operation ? `${entry.tool} ${entry.operation}` : entry.operation || "\u2014"}
      </div>
      <div className="font-mono text-[0.76rem] text-fg-muted truncate">{entry.scope || "\u2014"}</div>
      <div className={`font-mono text-[0.76rem] font-medium text-right ${riskColor}`}>
        {entry.risk_level || "\u2014"}
      </div>
    </div>
  );
}

function SectionHeader({ title }: { title: string }) {
  return (
    <div className="font-mono text-[0.72rem] font-medium tracking-widest uppercase text-accent mb-3">
      {title}
    </div>
  );
}

function formatBand(band: string): string {
  if (band === "insufficient_data") return "N/A";
  return band;
}

function bandColor(band?: string): string {
  if (!band) return "default";
  const b = band.toLowerCase();
  if (b === "excellent" || b === "good") return "green";
  if (b === "fair") return "amber";
  if (b === "poor") return "red";
  return "default";
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  } catch {
    return "\u2014";
  }
}
