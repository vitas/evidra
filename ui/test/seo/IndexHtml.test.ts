import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const TITLE = "Evidra — AI Infra Agent Evidence and Benchmarks";
const DESCRIPTION =
  "Evidra Bench is open source external regression testing for infrastructure agents and MCP tools, backed by evidence chains, behavioral signals, reliability scorecards, and benchmark reports.";
const KEYWORDS =
  "AI infrastructure agents, MCP evidence, infrastructure agent benchmarks, MCP regression testing, agent behavior reports, readiness reports, AI SRE benchmarks, evidence recorder";

function loadDocument() {
  const html = readFileSync(resolve(process.cwd(), "index.html"), "utf8");
  return new DOMParser().parseFromString(html, "text/html");
}

function metaContent(doc: Document, selector: string) {
  return doc.querySelector(selector)?.getAttribute("content");
}

describe("index.html SEO metadata", () => {
  it("reflects the Bench-first root positioning", () => {
    const doc = loadDocument();

    expect(doc.title).toBe(TITLE);
    expect(metaContent(doc, 'meta[name="description"]')).toBe(DESCRIPTION);
    expect(metaContent(doc, 'meta[name="keywords"]')).toBe(KEYWORDS);
    expect(metaContent(doc, 'meta[name="robots"]')).toBe("index, follow");
    expect(doc.querySelector('link[rel="canonical"]')?.getAttribute("href")).toBe("https://evidra.cc/");
    expect(metaContent(doc, 'meta[property="og:title"]')).toBe(TITLE);
    expect(metaContent(doc, 'meta[property="og:description"]')).toBe(
      "External regression testing for infrastructure agents and MCP tools with evidence-backed readiness reports.",
    );
    expect(metaContent(doc, 'meta[property="og:url"]')).toBe("https://evidra.cc/");
    expect(metaContent(doc, 'meta[property="og:type"]')).toBe("website");
    expect(metaContent(doc, 'meta[property="og:image"]')).toBe("https://bench.evidra.cc/og-bench.png");
    expect(metaContent(doc, 'meta[name="twitter:card"]')).toBe("summary_large_image");
    expect(metaContent(doc, 'meta[name="twitter:title"]')).toBe(TITLE);
    expect(metaContent(doc, 'meta[name="twitter:description"]')).toBe(
      "Run external regression tests for infrastructure agents and MCP tools, then compare evidence-backed reports.",
    );
    expect(metaContent(doc, 'meta[name="twitter:image"]')).toBe("https://bench.evidra.cc/og-bench.png");
  });
});
