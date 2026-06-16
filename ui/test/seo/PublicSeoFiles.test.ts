import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

function readPublicFile(path: string) {
  return readFileSync(resolve(process.cwd(), "public", path), "utf8");
}

function readUIFile(path: string) {
  return readFileSync(resolve(process.cwd(), path), "utf8");
}

describe("public SEO files", () => {
  it("exposes complete root metadata for the canonical domain", () => {
    const index = readUIFile("index.html");

    expect(index).toContain('<link rel="canonical" href="https://evidra.cc/"');
    expect(index).toContain('<link rel="icon" type="image/svg+xml" href="/favicon.svg"');
    expect(index).toContain('<meta property="og:url" content="https://evidra.cc/"');
    expect(index).toContain('<meta property="og:image" content="https://bench.evidra.cc/og-bench.png"');
    expect(index).toContain('<script type="application/ld+json">');
    expect(index).toContain("<noscript>");
    expect(index).toContain("AI infrastructure agent evidence");
  });

  it("serves a crawlable sitemap for the canonical domain", () => {
    const sitemap = readPublicFile("sitemap.xml");

    expect(sitemap).toContain('<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">');
    expect(sitemap).toContain("<loc>https://evidra.cc/</loc>");
  });

  it("points crawlers at the canonical sitemap", () => {
    const robots = readPublicFile("robots.txt");

    expect(robots).toContain("User-agent: *");
    expect(robots).toContain("Allow: /");
    expect(robots).toContain("Sitemap: https://evidra.cc/sitemap.xml");
  });

  it("ships favicon and web manifest assets", () => {
    expect(existsSync(resolve(process.cwd(), "public", "favicon.svg"))).toBe(true);

    const manifest = readPublicFile("site.webmanifest");
    expect(manifest).toContain('"name": "Evidra"');
    expect(manifest).toContain('"src": "/favicon.svg"');
  });
});
