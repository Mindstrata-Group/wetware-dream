import { test, expect } from "@playwright/test";
import { gotoApp } from "./helpers";

// Login page renders the Yandex button (or "not configured" message) and
// has dark-theme-safe CSS (no hardcoded white background that breaks in dark mode).
test("login page renders auth options", async ({ page }) => {
  await gotoApp(page, "/login");
  // Either we see the Yandex button OR the "not configured" message — both fine,
  // depending on env. The bug we guard against is a blank page or a 500.
  await expect
    .poll(async () => {
      const hasButton = await page.getByRole("link", { name: /Войти с Яндекс ID|Yandex/i }).count();
      const hasNotConfigured = await page.getByText(/Социальный вход пока не настроен/).count();
      return hasButton + hasNotConfigured;
    })
    .toBeGreaterThan(0);
});

test("login header has dark-theme-aware background (uses CSS variable, not hardcoded color)", async ({ page }) => {
  await gotoApp(page, "/login");
  // Force dark theme via localStorage hint (matches the script in layout.tsx).
  await page.evaluate(() => {
    localStorage.setItem("ms_theme", "dark");
    document.documentElement.dataset.theme = "dark";
  });
  await page.reload({ waitUntil: "domcontentloaded" });

  const header = page.locator("header").first();
  const bg = await header.evaluate((el) => getComputedStyle(el).backgroundColor);
  // Hardcoded light header was rgba(247, 250, 249, 0.9). In dark mode this
  // would still resolve to that exact value. After the fix it uses
  // color-mix on var(--card), which is dark in dark-theme.
  // We assert: background is NOT the bright (>240) light value.
  const match = bg.match(/\d+/g);
  if (match && match.length >= 3) {
    const [r, g, b] = match.map(Number);
    const brightness = (r + g + b) / 3;
    expect(brightness, `header bg too bright in dark mode: ${bg}`).toBeLessThan(200);
  }
});
