import type { ChangeDetail, EventItem } from "../types";
import { computeOverallStatus, computeTimeToDetect, formatRootCause } from "../utils/evidenceSummary";

type Props = {
  change: ChangeDetail | null;
  events: EventItem[];
  permalink: string;
  onCopyPermalink: () => void;
  onExport: () => void;
};

export function RootCauseBanner({ change, events, permalink, onCopyPermalink, onExport }: Props) {
  const overall = computeOverallStatus(change, events);
  const rootCause = formatRootCause(change, events);
  const ttd = computeTimeToDetect(change, events);

  return (
    <section className={`root-cause-banner status-${overall.status}`} data-testid="root-cause-banner">
      <div className="root-cause-banner__top">
        <div className="root-cause-banner__status-wrap">
          <span className={`root-cause-banner__status ${overall.status}`}>{overall.status}</span>
          <h2>Incident Summary</h2>
        </div>
        <div className="root-cause-banner__actions">
          <button type="button" className="secondary" onClick={onCopyPermalink} data-testid="copy-permalink-button">
            Copy permalink
          </button>
          <button type="button" className="secondary" onClick={onExport} data-testid="export-change-button">
            Export change
          </button>
        </div>
      </div>

      <div className="root-cause-banner__cause" data-testid="root-cause-text">
        <strong>Root cause</strong>
        <p className="root-cause-banner__primary">{rootCause.primaryText}</p>
        {rootCause.correlatorTokens.length > 0 ? (
          <div className="root-cause-banner__chips" aria-label="Root cause correlators">
            {rootCause.correlatorTokens.map((token) => (
              <span key={token} className="chip">{token}</span>
            ))}
          </div>
        ) : null}
        {rootCause.timestampText ? <p className="meta">Observed at {rootCause.timestampText}</p> : null}
      </div>

      <div className="root-cause-banner__meta">
        <span><strong>Time to detect:</strong> {ttd.label}</span>
      </div>

      <div className="meta root-cause-banner__link">Permalink: <code data-testid="change-permalink">{permalink}</code></div>
    </section>
  );
}
