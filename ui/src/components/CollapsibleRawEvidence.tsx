import { useState } from "react";

type Props = {
  payload: unknown;
  selected: boolean;
  detailAuthError: boolean;
};

export function CollapsibleRawEvidence({ payload, selected, detailAuthError }: Props) {
  const [open, setOpen] = useState(false);

  return (
    <section className="details" data-testid="raw-evidence">
      <button
        type="button"
        className="secondary"
        onClick={() => setOpen((v) => !v)}
        data-testid="toggle-raw-evidence"
      >
        {open ? "Hide raw evidence" : "Show raw evidence"}
      </button>
      {open ? (
        <pre data-testid="payload-viewer">
          {selected
            ? JSON.stringify(payload, null, 2)
            : detailAuthError
              ? "Not authorized to view evidence for this change."
              : "Select a change to view raw evidence."}
        </pre>
      ) : (
        <p className="meta">Raw payload is hidden by default. Expand only for low-level inspection.</p>
      )}
    </section>
  );
}
