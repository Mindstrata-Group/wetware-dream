import { expect, test } from "@playwright/test";

// Static sites (tir, completely-different) live in their own repos and run
// their own UI E2E. This repo still owns what sits in front of them: Caddy
// routes, the /srv/static mount and the /api proxy on each subdomain. These
// checks catch a Caddyfile or compose change that leaves a subdomain empty,
// serving the old in-container path, or leaking the tir answer banks.

function siteURL(sub: string, path: string): string {
  const base = new URL(process.env.PLAYWRIGHT_BASE_URL ?? "https://stage.mindstrata.ru");
  if (base.hostname === "stage.mindstrata.ru" || base.hostname === "mindstrata.ru") {
    base.hostname = `${sub}.${base.hostname}`;
  }
  return new URL(path, base.origin).toString();
}

test.describe("static sites behind Caddy", () => {
  test("tir: pages served from a release, answers never public, API proxied", async ({ request }) => {
    for (const path of ["/", "/praktika2.html", "/sr/player.js"]) {
      const res = await request.get(siteURL("tir", path));
      expect(res.status(), path).toBe(200);
    }
    // release.txt exists only in trees deployed by the tir repo into
    // /srv/static; the old baked-in directory never had it.
    const rel = await request.get(siteURL("tir", `/release.txt?t=${Date.now()}`));
    expect(rel.status()).toBe(200);
    expect((await rel.text()).trim()).toMatch(/^\d{8}T\d{6}Z-[0-9a-f]{7,}$/);

    for (const path of ["/data/quest-2-1.json", "/data/task-1-1.json"]) {
      const res = await request.get(siteURL("tir", path));
      expect(res.status(), `${path} must not be served: it holds the answers`).toBe(404);
    }

    // Public read-only endpoint the tir stats page uses; proves /api/* on the
    // subdomain still reaches the Mindstrata API.
    const stats = await request.get(siteURL("tir", "/api/game/stats?game=praktika-2"));
    expect(stats.status()).toBe(200);
    expect((await stats.json()).ok).toBe(true);
  });

  test("completely-different: map and its data are served", async ({ request }) => {
    for (const path of ["/", "/app.js", "/data/graph_mobile.json"]) {
      const res = await request.get(siteURL("completely-different", path));
      expect(res.status(), path).toBe(200);
    }
    const rel = await request.get(siteURL("completely-different", `/release.txt?t=${Date.now()}`));
    expect(rel.status()).toBe(200);
  });
});
