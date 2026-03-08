import { useEffect, useRef, useState } from "react";
import mermaid from "mermaid";

interface MermaidDiagramProps {
  chart: string;
  className?: string;
}

let mermaidCounter = 0;

export function MermaidDiagram({ chart, className = "" }: MermaidDiagramProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [svg, setSvg] = useState("");
  const idRef = useRef(`mermaid-${++mermaidCounter}`);

  useEffect(() => {
    const theme = document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "default";
    mermaid.initialize({ startOnLoad: false, theme, securityLevel: "loose" });

    // Use a unique ID per render to avoid conflicts
    const id = `${idRef.current}-${Date.now()}`;
    mermaid.render(id, chart).then(({ svg: rendered }) => {
      setSvg(rendered);
    }).catch(() => {
      // Retry once — mermaid can fail on first render in some environments
      setTimeout(() => {
        const retryId = `${idRef.current}-retry-${Date.now()}`;
        mermaid.render(retryId, chart).then(({ svg: rendered }) => {
          setSvg(rendered);
        }).catch(() => {});
      }, 100);
    });
  }, [chart]);

  // Re-render when theme changes
  useEffect(() => {
    const observer = new MutationObserver(() => {
      const theme = document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "default";
      mermaid.initialize({ startOnLoad: false, theme, securityLevel: "loose" });

      const id = `${idRef.current}-theme-${Date.now()}`;
      mermaid.render(id, chart).then(({ svg: rendered }) => {
        setSvg(rendered);
      }).catch(() => {});
    });

    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    return () => observer.disconnect();
  }, [chart]);

  return (
    <div
      ref={ref}
      className={`flex justify-center bg-bg-elevated border border-border rounded-[10px] p-8 overflow-x-auto shadow-[inset_0_1px_3px_rgba(5,80,60,0.06)] ${className}`}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
