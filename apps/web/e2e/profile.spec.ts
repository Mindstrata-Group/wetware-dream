import { test, expect, type Page, type Route } from "@playwright/test";
import { gotoApp } from "./helpers";

// Profile page E2E with a mocked backend via page.route().
// Needs no live DB; isolated from staging state.
// Covers: profile loading, the delete modal, the theme.
//
// Important: Next.js middleware checks mindstrata_session on the server,
// so a fake cookie must be set BEFORE goto, otherwise it redirects to /login.
// page.route() intercepts only browser fetch requests, not edge middleware.

const MOCK_PROFILE = {
  ok: true,
  user: { id: 42, email: "user@example.com", role: "user", status: "active" },
  allowMessageAnonymization: true,
  hasAccess: true,
  activeTo: new Date(Date.now() + 30 * 24 * 3600 * 1000).toISOString(),
  activeModes: [{ modeId: 10, modeName: "Базовый режим", activeTo: null }],
  stats: { dialogs: 5, messages: 120, liveMessages: 30, tokens: 45000 },
};

async function setupProfile(page: Page) {
  // Fake session cookie: lets us pass the middleware without a real login.
  const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000";
  const domain = new URL(baseURL).hostname;
  await page.context().addCookies([{
    name: "mindstrata_session",
    value: "e2e-fake-session-for-profile-tests",
    domain,
    path: "/",
    httpOnly: false,
    secure: baseURL.startsWith("https"),
    sameSite: "Lax",
  }]);

  // Mock /api/profile: GET returns the profile, DELETE simulates deletion.
  await page.route("**/api/profile", (route: Route) => {
    if (route.request().method() === "GET") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_PROFILE),
      });
    }
    if (route.request().method() === "DELETE") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true }),
      });
    }
    return route.continue();
  });
}

test.describe("profile page (mocked API)", () => {
  test("загружает профиль и отображает email", async ({ page }) => {
    await setupProfile(page);
    await gotoApp(page, "/profile");

    // email appears twice (avatar + table); take the first one to avoid strict mode
    await expect(page.getByText("user@example.com").first()).toBeVisible({ timeout: 8000 });
  });

  test("показывает активный режим доступа", async ({ page }) => {
    await setupProfile(page);
    await gotoApp(page, "/profile");

    await expect(page.getByText("Базовый режим")).toBeVisible({ timeout: 8000 });
  });

  test("кнопка 'Удалить аккаунт' открывает модалку", async ({ page }) => {
    await setupProfile(page);
    await gotoApp(page, "/profile");

    await page.getByRole("button", { name: /Удалить аккаунт/i }).click();
    await expect(page.getByText("Это действие необратимо")).toBeVisible({ timeout: 5000 });
  });

  test("кнопка Отмена закрывает модалку без удаления", async ({ page }) => {
    await setupProfile(page);
    await gotoApp(page, "/profile");

    await page.getByRole("button", { name: /Удалить аккаунт/i }).click();
    await expect(page.getByText("Это действие необратимо")).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Отмена/i }).click();
    await expect(page.getByText("Это действие необратимо")).not.toBeVisible({ timeout: 3000 });
  });

  test("подтверждение удаления → редирект на /login?deleted=1", async ({ page }) => {
    await setupProfile(page);
    await gotoApp(page, "/profile");

    await page.getByRole("button", { name: /Удалить аккаунт/i }).click();
    await expect(page.getByText("Это действие необратимо")).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Удалить навсегда/i }).click();
    await page.waitForURL(/\/login/, { timeout: 8000 });
    expect(page.url()).toContain("deleted=1");
  });

  test("переключатель темы меняет data-theme на documentElement", async ({ page }) => {
    await setupProfile(page);
    await gotoApp(page, "/profile");

    await expect(page.getByRole("button", { name: /Тёмная/ })).toBeVisible({ timeout: 8000 });
    await page.getByRole("button", { name: /Тёмная/ }).click();

    const theme = await page.evaluate(() => document.documentElement.dataset.theme);
    expect(theme).toBe("dark");
  });

  test("настройка анонимизации объясняет эффект и сохраняет выбор", async ({ page }) => {
    await setupProfile(page);

    let savedValue: boolean | null = null;
    await page.route("**/api/profile/settings", async (route: Route) => {
      const payload = route.request().postDataJSON() as { allowMessageAnonymization?: boolean };
      savedValue = payload.allowMessageAnonymization ?? null;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, allowMessageAnonymization: savedValue }),
      });
    });

    await gotoApp(page, "/profile");

    await expect(page.getByText(/Хотите анонимно общаться - оставьте включенным/i)).toBeVisible({ timeout: 8000 });
    await expect(page.getByText(/Хотите точнее - выключите/i)).toBeVisible();

    const toggle = page.getByRole("switch", { name: /Убирать личные данные из старой переписки/i });
    await expect(toggle).toHaveAttribute("aria-checked", "true");

    await toggle.click();

    await expect.poll(() => savedValue).toBe(false);
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect(page.getByText(/обращения будут точнее/i)).toBeVisible();
  });
});
