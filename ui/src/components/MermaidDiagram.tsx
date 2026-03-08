import { useEffect, useRef, useId } from "react";
import mermaid from "mermaid";

interface MermaidDiagramProps {
  chart: string;
  className?: string;
}

export function MermaidDiagram({ chart, className = "" }: MermaidDiagramProps) {
  const ref = useRef<HTMLDivElement>(null);
  const id = useId().replace(/:/g, "_");

  useEffect(() => {
    if (!ref.current) return;

    const theme = document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "default";
    mermaid.initialize({ startOnLoad: false, theme, securityLevel: "loose" });

    mermaid.render(`mermaid-${id}`, chart).then(({ svg }) => {
      if (ref.current) ref.current.innerHTML = svg;
    });
  }, [chart, id]);

  return (
    <div
      ref={ref}
      className={`flex justify-center bg-bg-elevated border border-border rounded-[10px] p-8 overflow-x-auto ${className}`}
    />
  );
}
