import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const TITLE = "Evidra — Verifiable Evidence for MCP Operations";
const DESCRIPTION =
  "Open-source MCP execution evidence that reconciles what an agent declared, what the proxy observed, and what the agent reported.";

function loadDocument() {
  const html = readFileSync(resolve(process.cwd(), "index.html"), "utf8");
  return new DOMParser().parseFromString(html, "text/html");
}

function uniqueAttribute(doc: Document, selector: string, attribute: string) {
  const elements = doc.querySelectorAll(selector);
  expect(elements).toHaveLength(1);
  return elements[0]?.getAttribute(attribute);
}

function metaContent(doc: Document, selector: string) {
  return uniqueAttribute(doc, selector, "content");
}

function jsonLdag(doc: Document) {
  const script = doc.querySelector('script[type="application/ld+json"]');
  return JSON.parse(script?.textContent ?? '{"@graph":[]}');
}

describe("index.html SEO metadata", () => {
  it("reflects the current root metadata", () => {
    const doc = loadDocument();

    expect(doc.title).toBe(TITLE);
    expect(metaContent(doc, 'meta[name="description"]')).toBe(DESCRIPTION);
    expect(metaContent(doc, 'meta[name="robots"]')).toBe("index, follow");
    expect(uniqueAttribute(doc, 'link[rel="canonical"]', "href")).toBe(
      "https://evidra.cc/",
    );
    expect(metaContent(doc, 'meta[property="og:title"]')).toBe(TITLE);
    expect(metaContent(doc, 'meta[property="og:description"]')).toBe(
      DESCRIPTION,
    );
    expect(metaContent(doc, 'meta[property="og:url"]')).toBe(
      "https://evidra.cc/",
    );
    expect(metaContent(doc, 'meta[property="og:type"]')).toBe("website");
    expect(metaContent(doc, 'meta[property="og:image"]')).toBe(
      "https://evidra.cc/og/evidra-core.png",
    );
    expect(metaContent(doc, 'meta[property="og:image:width"]')).toBe("1200");
    expect(metaContent(doc, 'meta[property="og:image:height"]')).toBe("630");
    expect(metaContent(doc, 'meta[property="og:image:alt"]')).toBe(
      "Evidra — verifiable MCP execution evidence",
    );
    expect(metaContent(doc, 'meta[name="twitter:card"]')).toBe(
      "summary_large_image",
    );
    expect(metaContent(doc, 'meta[name="twitter:image"]')).toBe(
      "https://evidra.cc/og/evidra-core.png",
    );
    expect(metaContent(doc, 'meta[name="twitter:image:alt"]')).toBe(
      "Evidra — verifiable MCP execution evidence",
    );
    expect(metaContent(doc, 'meta[name="twitter:title"]')).toBe(TITLE);
    expect(metaContent(doc, 'meta[name="twitter:description"]')).toBe(
      DESCRIPTION,
    );
  });

  it("rejects duplicate metadata selectors", () => {
    const doc = new DOMParser().parseFromString(
      '<meta property="og:image" content="one"><meta property="og:image" content="two">',
      "text/html",
    );

    expect(() => metaContent(doc, 'meta[property="og:image"]')).toThrow();
  });

  it("lets Core, not Bench, own the metadata", () => {
    const html = readFileSync(resolve(process.cwd(), "index.html"), "utf8");

    expect(html).not.toContain("bench.evidra.cc");
    expect(html.toLowerCase()).not.toContain("benchmark");
    expect(html).not.toContain("name=\"keywords\"");
  });

  it("keeps no route of the removed hosted application", () => {
    const html = readFileSync(resolve(process.cwd(), "index.html"), "utf8");

    expect(html).not.toMatch(/\/onboarding/);
    expect(html).not.toMatch(/\/dashboard/);
    expect(html).not.toMatch(/href="\/evidence/);
    expect(html).not.toMatch(/\/docs\/api/);
  });

  it("describes one SoftwareApplication and no hosted service", () => {
    const graph = jsonLdag(loadDocument())["@graph"];

    const applications = graph.filter((n: { "@type": string }) =>
      n["@type"].includes("SoftwareApplication"),
    );
    const services = graph.filter((n: { "@type": string }) =>
      n["@type"].includes("Service"),
    );
    expect(applications).toHaveLength(1);
    expect(services).toHaveLength(0);
  });

  it("noscript explains Core and links to getting started and GitHub", () => {
    const doc = loadDocument();
    const noscript = doc.querySelector("noscript");
    expect(noscript).not.toBeNull();
    const text = noscript?.textContent ?? "";
    expect(text).toMatch(/execution evidence/i);
    const hrefs = Array.from(noscript?.querySelectorAll("a") ?? []).map((a) =>
      a.getAttribute("href"),
    );
    expect(hrefs).toContain(
      "https://github.com/vitas/evidra/blob/main/docs/getting-started.md",
    );
    expect(hrefs).toContain("https://github.com/vitas/evidra");
  });
});
