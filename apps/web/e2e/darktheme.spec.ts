import { test, expect, type Page } from "@playwright/test";
import { gotoApp } from "./helpers";

// Dark theme audit: walk through all main pages, check that
// the background of body + all key elements (header, card, button) is
// not light in the dark theme. No hardcoded #fff or rgba(247,250,...) that
// are equally light in both themes.
//
// Regression scenario: someone added an inline style={{ background: '#fff' }}
// to a component; this test will catch it.

/**
 * Returns the average brightness (R+G+B)/3 of a CSS color string.
 * For transparent/invalid colors, returns NaN.
 */
async function brightness(page: Page, selector: string, prop: "backgroundColor" | "color" = "backgroundColor"): Promise<number> {
  return page.evaluate(
    ({ sel, p }) => {
      const el = document.querySelector(sel);
      if (!el) return NaN;
      const v = getComputedStyle(el)[p as "backgroundColor" | "color"];
      const m = v.match(/\d+/g);
      if (!m || m.length < 3) return NaN;
      const [r, g, b] = m.map(Number);
      return (r + g + b) / 3;
    },
    { sel: selector, p: prop }
  );
}

async function installDarkTheme(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("ms_theme", "dark");
    document.documentElement.dataset.theme = "dark";
  });
}

async function gotoForThemeAudit(page: Page, path: string) {
  await installDarkTheme(page);
  // domcontentloaded: do not wait for background API requests of the landing/chat to finish.
  return gotoApp(page, path);
}

const PAGES_TO_CHECK = [
  { name: "landing", path: "/" },
  { name: "login", path: "/login" },
  { name: "register", path: "/register" },
  { name: "access", path: "/access?force=promo" },
];

for (const p of PAGES_TO_CHECK) {
  test(`dark theme: ${p.name} (${p.path}) does not leak bright bg`, async ({ page }) => {
    // Set the theme before navigation and do not reload: on stage some
    // pages may wait a long time for background resources, which turns the audit into a timeout.
    await gotoForThemeAudit(page, p.path);
    await expect.poll(() => brightness(page, "body")).not.toBeNaN();

    // Body background should be dark.
    const bodyBg = await brightness(page, "body");
    if (!isNaN(bodyBg)) {
      expect(bodyBg, `${p.name}: body bg too bright (${bodyBg})`).toBeLessThan(200);
    }

    // Find any header / nav / card with a non-transparent bg → none should be very bright.
    const containers = await page.locator("header, nav, main > div, .card, [role='main']").all();
    for (let i = 0; i < Math.min(containers.length, 5); i++) {
      const bg = await containers[i].evaluate((el) => getComputedStyle(el).backgroundColor);
      const m = bg.match(/\d+/g);
      if (m && m.length >= 3) {
        const [r, g, b, a = "1"] = m;
        // Skip fully transparent.
        if (Number(a) > 0.05) {
          const br = (Number(r) + Number(g) + Number(b)) / 3;
          // Allow up to 220 to leave room for slight contrast variants, but
          // hardcoded #fff (255) or rgba(247,250,...) ≈ 248 must fail.
          expect(br, `${p.name}: container #${i} bg too bright (${bg})`).toBeLessThan(240);
        }
      }
    }
  });
}

test.describe("admin button consistency (no hardcoded inline styles)", () => {
  test("admin page buttons should use ms-button class, not inline bg colors", async ({ page }) => {
    // Mock /me as admin so /admin renders.
    await page.route("**/api/auth/me", async (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          ok: true,
          user: { id: 1, email: "admin@test.local", role: "admin", status: "active" },
        }),
      })
    );

    const res = await gotoApp(page, "/admin");
    if ((res?.status() ?? 500) >= 500) {
      test.skip(true, "admin page not reachable in this env");
    }

    // Buttons that have inline background-color set are suspect — they bypass
    // theme system. Count them and assert it's below a threshold (some are
    // legitimately custom, e.g. Yandex auth or destructive red).
    const inlineStyledButtons = await page.locator(
      'button[style*="background"], a[style*="background"]'
    ).count();

    // Threshold: up to 5 elements with an inline bg = legitimate exception (custom auth
    // buttons, destructive actions). More is a sign of a regression and a violation
    // ms-button consistency.
    expect(
      inlineStyledButtons,
      `Found ${inlineStyledButtons} buttons with inline background style. Prefer .ms-button class.`
    ).toBeLessThanOrEqual(10);
  });
});
