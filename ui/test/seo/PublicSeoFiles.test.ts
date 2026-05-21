import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

function readPublicFile(path: string) {
  return readFileSync(resolve(process.cwd(), "public", path), "utf8");
}

describe("public SEO files", () => {
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
});
