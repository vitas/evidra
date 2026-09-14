import { useEffect, useRef, useState } from "react";

interface CodeBlockProps {
  code: string;
  className?: string;
}

export function CodeBlock({ code, className = "" }: CodeBlockProps) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => () => clearTimeout(timer.current), []);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(code);
      setStatus("copied");
    } catch {
      setStatus("failed");
    }
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setStatus("idle"), 2000);
  }

  return (
    <div className={`codeblock ${className}`}>
      <div className="codeblock-bar">
        <span className="codeblock-status" role="status" aria-live="polite">
          {status === "copied" && "copied"}
          {status === "failed" && "copy failed"}
        </span>
        <button
          type="button"
          onClick={handleCopy}
          aria-label="Copy code"
          className="codeblock-copy"
        >
          copy
        </button>
      </div>
      <pre className="codeblock-body">{code}</pre>
    </div>
  );
}
