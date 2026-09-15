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
    expect(index).toContain(
      '<link rel="icon" type="image/svg+xml" href="/favicon.svg"',
    );
    expect(index).toContain('<script type="application/ld+json">');
    expect(index).toContain("<noscript>");
  });

  it("serves a sitemap with only the canonical root", () => {
    const sitemap = readPublicFile("sitemap.xml");

    expect(sitemap).toContain('<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">');
    const locs = sitemap.match(/<loc>[^<]*<\/loc>/g) ?? [];
    expect(locs).toEqual(["<loc>https://evidra.cc/</loc>"]);
    expect(sitemap).not.toMatch(/\/onboarding|\/dashboard|\/evidence/);
  });

  it("points crawlers at the canonical sitemap", () => {
    const robots = readPublicFile("robots.txt");

    expect(robots).toContain("User-agent: *");
    expect(robots).toContain("Allow: /");
    expect(robots).toContain("Sitemap: https://evidra.cc/sitemap.xml");
  });

  it("ships favicon and a Core-styled web manifest", () => {
    expect(existsSync(resolve(process.cwd(), "public", "favicon.svg"))).toBe(
      true,
    );

    const manifest = readPublicFile("site.webmanifest");
    expect(manifest).toContain('"name": "Evidra"');
    expect(manifest).toContain('"src": "/favicon.svg"');
    expect(manifest).toMatch(/execution evidence/i);
    expect(manifest).not.toContain("reliability reports");
  });

  it("ships no OpenAPI or Swagger UI for a service that does not exist", () => {
    expect(existsSync(resolve(process.cwd(), "public", "openapi.yaml"))).toBe(
      false,
    );
    expect(
      existsSync(resolve(process.cwd(), "public", "docs", "api", "index.html")),
    ).toBe(false);
  });
});
