import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const TITLE = "Evidra — AI Infra Agent Evidence and Benchmarks";
const DESCRIPTION =
  "Evidra Bench provides external regression testing for infrastructure agents and MCP tools. Evidra OSS records agent actions and outcomes as evidence for readiness reports, failure analysis, and public benchmarks.";
const KEYWORDS =
  "AI infrastructure agents, MCP benchmarks, infrastructure agent regression testing, MCP tools, readiness reports, failure autopsy, public leaderboard, evidence recorder";

function loadDocument() {
  const html = readFileSync(resolve(process.cwd(), "index.html"), "utf8");
  return new DOMParser().parseFromString(html, "text/html");
}

function metaContent(doc: Document, selector: string) {
  return doc.querySelector(selector)?.getAttribute("content");
}

describe("index.html SEO metadata", () => {
  it("reflects the Bench-first positioning", () => {
    const doc = loadDocument();

    expect(doc.title).toBe(TITLE);
    expect(metaContent(doc, 'meta[name="description"]')).toBe(DESCRIPTION);
    expect(metaContent(doc, 'meta[name="keywords"]')).toBe(KEYWORDS);
    expect(metaContent(doc, 'meta[name="robots"]')).toBe("index, follow");
    expect(metaContent(doc, 'meta[property="og:title"]')).toBe(TITLE);
    expect(metaContent(doc, 'meta[property="og:description"]')).toBe(DESCRIPTION);
    expect(metaContent(doc, 'meta[property="og:type"]')).toBe("website");
    expect(metaContent(doc, 'meta[name="twitter:card"]')).toBe("summary");
    expect(metaContent(doc, 'meta[name="twitter:title"]')).toBe(TITLE);
    expect(metaContent(doc, 'meta[name="twitter:description"]')).toBe(DESCRIPTION);
  });
});
