import type { EvidenceSummaryGroup } from "../ux";

type Props = {
  groups: EvidenceSummaryGroup[];
};

export function EvidenceSummary({ groups }: Props) {
  return (
    <section className="evidence-summary" data-testid="evidence-summary">
      <h2>Evidence Summary</h2>
      {groups.length === 0 ? (
        <p className="meta">Select a change to view summarized evidence.</p>
      ) : (
        <div className="summary-groups">
          {groups.map((group) => (
            <article key={group.title} className="summary-group">
              <h3>{group.title}</h3>
              <ul>
                {group.items.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
