import { useEffect, useMemo, useRef, useState } from "react";

import { formatSubjectDisplay } from "../utils/subjectDisplay";

type SubjectSelectProps = {
  value: string;
  options: string[];
  onChange: (next: string) => void;
};

export function SubjectSelect({ value, options, onChange }: SubjectSelectProps) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  useEffect(() => {
    function handleOutsideClick(e: MouseEvent) {
      if (!rootRef.current?.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    window.addEventListener("mousedown", handleOutsideClick);
    return () => window.removeEventListener("mousedown", handleOutsideClick);
  }, []);

  const selected = value ? formatSubjectDisplay(value) : null;
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((raw) => {
      const f = formatSubjectDisplay(raw);
      return (
        raw.toLowerCase().includes(q) ||
        f.title.toLowerCase().includes(q) ||
        f.subtitle.toLowerCase().includes(q)
      );
    });
  }, [options, query]);

  return (
    <div className="subject-select" ref={rootRef} data-testid="subject-select">
      <button
        type="button"
        className="subject-select-trigger"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        {selected ? (
          <>
            <span className="subject-title">{selected.title}</span>
            <span className="subject-subtitle">{selected.subtitle}</span>
          </>
        ) : (
          <span className="subject-placeholder">Select subject</span>
        )}
      </button>
      {open ? (
        <div className="subject-select-menu" role="listbox">
          <input
            className="subject-search"
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search app or namespace"
            data-testid="subject-search"
          />
          <div className="subject-options">
            {filtered.map((raw) => {
              const display = formatSubjectDisplay(raw);
              return (
                <button
                  type="button"
                  key={raw}
                  role="option"
                  className={`subject-option ${raw === value ? "selected" : ""}`.trim()}
                  onClick={() => {
                    onChange(raw);
                    setOpen(false);
                  }}
                  title={raw}
                >
                  <span className="subject-title">{display.title}</span>
                  <span className="subject-subtitle">{display.subtitle}</span>
                </button>
              );
            })}
            {filtered.length === 0 ? (
              <div className="subject-empty">No subjects match.</div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

