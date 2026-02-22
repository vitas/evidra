import type { EventItem } from "../types";
import { categorizeEvent, extractEventStatus, humanizeEventType, sortEventsChronologically } from "../utils/evidenceSummary";

type Props = {
  events: EventItem[];
  selectedEventID: string;
  onSelect: (id: string) => void;
  breakingEventID: string;
};

export function IncidentTimeline({ events, selectedEventID, onSelect, breakingEventID }: Props) {
  const sorted = sortEventsChronologically(events);

  if (sorted.length === 0) {
    return (
      <section className="incident-timeline">
        <h2>Incident Timeline</h2>
        <p className="meta">No timeline events recorded for this change.</p>
      </section>
    );
  }

  return (
    <section className="incident-timeline">
      <h2>Incident Timeline</h2>
      <ul id="eventList" className="timeline-list" data-testid="timeline-list">
        {sorted.map((event, idx) => {
          const category = categorizeEvent(event);
          const eventStatus = extractEventStatus(event);
          const isBreaking = breakingEventID !== "" && event.id === breakingEventID;
          const isSelected = selectedEventID === event.id;
          return (
            <li
              key={event.id}
              className={`timeline-item ${isSelected ? "selected" : ""} ${isBreaking ? "breaking" : ""}`.trim()}
              onClick={() => onSelect(event.id)}
              data-testid="timeline-item"
            >
              <div className="timeline-item__marker" aria-hidden="true">
                <span className="timeline-item__node" />
                {idx === sorted.length - 1 ? <span className="timeline-item__end-mask" /> : null}
              </div>

              <div className="timeline-item__content">
                <div className="timeline-item__row">
                  <time className="meta">{fmtDate(event.time)}</time>
                  <span className={`chip category ${category}`}>{category}</span>
                  {eventStatus ? <span className="chip status-chip">{eventStatus}</span> : null}
                  {isBreaking ? <span className="chip break-flag">breaking point</span> : null}
                </div>

                <div className="timeline-item__row">
                  <strong>{humanizeEventType(event.type)}</strong>
                  <span className="meta">{event.source || "unknown source"}</span>
                </div>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function fmtDate(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toISOString().replace("T", " ").replace("Z", " UTC");
}
