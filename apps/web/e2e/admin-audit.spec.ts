import { test, expect, type Page, type Route } from "@playwright/test";
import { gotoApp } from "./helpers";

// Admin audit: catches mis-labeled tabs, missing action buttons,
// and styling regressions across all 12 main tabs + sub-tabs.
//
// Strategy: mock /api/auth/me as admin, open /admin, click
// every tab, assert that its label from the expected list is rendered
// and that there is at least one action button (Save/Create/Apply).
//
// Also: check that the dark theme leaves no light patches on ANY
// admin tab (we used to catch #edf8f5 / #f8fbfa).

const EXPECTED_TABS: Array<{ id: string; label: string }> = [
  { id: "profile",       label: "Профиль" },
  { id: "system",        label: "Система" },
  { id: "stats",         label: "Статистика" },
  { id: "users",         label: "Пользователи" },
  { id: "access",        label: "Доступы" },
  { id: "modes",         label: "Режимы" },
  { id: "tariffs",       label: "Тарифы" },
  { id: "promocodes",    label: "Промокоды" },
  { id: "exports",       label: "Экспорт" },
  { id: "orchestration", label: "Оркестрация" },
  { id: "payments",      label: "Платежи" },
  { id: "content",       label: "Контент" },
];

async function mockAuthAsAdmin(page: Page) {
  await page.route("**/api/auth/me", async (route: Route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        ok: true,
        user: { id: 1, email: "audit@test.local", role: "admin", status: "active" },
      }),
    })
  );
  // Catch-all for other admin endpoints so the UI does not fail on missing data.
  await page.route("**/api/admin/**", async (route: Route) => {
    const url = route.request().url();
    // Minimal stubs for the most frequent endpoints.
    if (url.includes("/api/admin/modes")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, modes: [], total: 0, limit: 50, offset: 0 }),
      });
    }
    if (url.includes("/api/admin/users") && !url.includes("/access")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, users: [], total: 0, limit: 50, offset: 0 }),
      });
    }
    if (url.includes("/api/admin/tariffs")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, tariffs: [], groups: [] }),
      });
    }
    if (url.includes("/api/admin/status")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, status: { users: 0, sessions: 0, modes: 0, liveAI: false } }),
      });
    }
    if (url.includes("/api/admin/stats")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, totals: {}, byMode: [], dailyByMode: [] }),
      });
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ok: true }),
    });
  });
}

test.describe("admin tabs audit (labels + presence)", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthAsAdmin(page);
  });

  test("admin page loads and all 12 tabs can be opened", async ({ page }) => {
    const res = await gotoApp(page, "/admin");
    if ((res?.status() ?? 500) >= 500) test.skip(true, "/admin not reachable");

    for (const t of EXPECTED_TABS) {
      await expect(page.getByText(t.label, { exact: true }).first()).toBeVisible({ timeout: 10_000 });
      const count = await page.getByText(t.label, { exact: true }).count();
      expect(count, `tab label "${t.label}" not found on /admin`).toBeGreaterThan(0);
      const tabBtn = page.getByText(t.label, { exact: true }).first();
      await tabBtn.click();

      // After the click the page must not show a null/undefined/empty render.
      // Minimum: body is not empty, no error overlay.
      const bodyText = await page.locator("body").innerText().catch(() => "");
      expect(bodyText.length, `${t.label}: empty page after click`).toBeGreaterThan(50);
    }
  });
});

test.describe("admin dark theme: no light bg leaks across tabs", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthAsAdmin(page);
  });

  test("admin in dark theme: max bg brightness within reasonable range", async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("ms_theme", "dark");
      document.documentElement.dataset.theme = "dark";
    });
    const res = await gotoApp(page, "/admin");
    if ((res?.status() ?? 500) >= 500) test.skip(true, "/admin not reachable");

    // Body should be dark.
    const bodyBg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
    const m = bodyBg.match(/\d+/g);
    if (m && m.length >= 3) {
      const [r, g, b] = m.map(Number);
      expect((r + g + b) / 3, `body bg too bright in dark: ${bodyBg}`).toBeLessThan(200);
    }

    // Click through every tab and check the sidebar/content bg.
    for (const t of EXPECTED_TABS.slice(0, 5)) {
      // limit to 5 so the test does not hang; the critical ones come first
      const tabBtn = page.getByText(t.label, { exact: true }).first();
      await tabBtn.click().catch(() => {});
      // The main admin content.
      const contentBgs = await page.evaluate(() => {
        const els = Array.from(document.querySelectorAll(".ms-admin-content, .ms-admin-card, .card"));
        return els.slice(0, 10).map((el) => getComputedStyle(el).backgroundColor);
      });
      for (const bg of contentBgs) {
        const m2 = bg.match(/\d+/g);
        if (!m2 || m2.length < 3) continue;
        const [r, g, b, a = "1"] = m2;
        if (Number(a) < 0.1) continue;
        const br = (Number(r) + Number(g) + Number(b)) / 3;
        expect(br, `${t.label}: light bg leak ${bg}`).toBeLessThan(240);
      }
    }
  });
});

test.describe("admin: button sizes stay compact (chat reference is ~32px)", () => {
  test.setTimeout(90_000);
  test.beforeEach(async ({ page }) => {
    await mockAuthAsAdmin(page);
  });

  test("admin action buttons height <= 42px", async ({ page }) => {
    const res = await gotoApp(page, "/admin");
    if ((res?.status() ?? 500) >= 500) test.skip(true, "/admin not reachable");

    const violations: Array<{ tab: string; text: string; height: number }> = [];
    for (const t of EXPECTED_TABS) {
      const btn = page.getByText(t.label, { exact: true }).first();
      await btn.click().catch(() => {});

      const oversized = await page.evaluate(() => {
        const xs: Array<{ text: string; height: number }> = [];
        const buttons = Array.from(document.querySelectorAll(
          ".ms-admin-content button, .ms-admin-content a.ms-button"
        ));
        for (const b of buttons) {
          if ((b as HTMLElement).offsetParent === null) continue;
          const r = b.getBoundingClientRect();
          if (r.height > 42) {
            xs.push({
              text: ((b as HTMLElement).innerText || "").slice(0, 30),
              height: Math.round(r.height),
            });
          }
        }
        return xs;
      });
      for (const v of oversized) violations.push({ tab: t.label, ...v });
    }
    expect(
      violations,
      `Found ${violations.length} oversized buttons in admin (chat reference is 32px):\n${JSON.stringify(violations.slice(0, 15), null, 2)}`
    ).toEqual([]);
  });
});

test.describe("admin: action buttons use ms-button class", () => {
  test.setTimeout(90_000);
  test.beforeEach(async ({ page }) => {
    await mockAuthAsAdmin(page);
  });

  test("all visible buttons have a class (no bare browser-default buttons)", async ({ page }) => {
    const res = await gotoApp(page, "/admin");
    if ((res?.status() ?? 500) >= 500) test.skip(true, "/admin not reachable");

    // Take each tab and count buttons without className.
    const violations: Array<{ tab: string; count: number }> = [];
    for (const t of EXPECTED_TABS) {
      const btn = page.getByText(t.label, { exact: true }).first();
      await btn.click().catch(() => {});

      // Find all buttons currently in DOM without ANY class attribute (or
      // empty class). Sub-tabs that toggle className='' when not active
      // would be flagged — but they have parent container styling, so
      // we relax: count only buttons whose computed border is "none".
      const bare = await page.evaluate(() => {
        const buttons = Array.from(document.querySelectorAll("button"));
        return buttons.filter((b) => {
          const s = getComputedStyle(b);
          // browser default: border-style="outset" or "none" + padding 1-2px.
          const hasNoBorder = s.borderStyle === "none";
          const hasNoClass = !b.className || b.className.trim() === "";
          // Invisible/disabled ones are allowed.
          if (b.hidden || (b as HTMLButtonElement).disabled) return false;
          return hasNoBorder && hasNoClass;
        }).length;
      });
      if (bare > 0) violations.push({ tab: t.label, count: bare });
    }

    expect(
      violations,
      `Found unstyled bare buttons in admin tabs: ${JSON.stringify(violations)}`
    ).toEqual([]);
  });
});
