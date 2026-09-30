import { test, expect, type Page, type Route } from "@playwright/test";
import { gotoApp } from "./helpers";

function corsHeaders() {
  return {
    "Access-Control-Allow-Origin": new URL(process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000").origin,
    "Access-Control-Allow-Credentials": "true",
    "Access-Control-Allow-Headers": "content-type",
    "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
  };
}

async function mockSiteContent(page: Page, content: Record<string, unknown>) {
  await page.route("**/api/public/site-content", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      headers: corsHeaders(),
      body: JSON.stringify({ ok: true, content }),
    });
  });
}

async function mockGuestProfile(page: Page) {
  await page.route("**/api/profile", async (route: Route) => {
    await route.fulfill({
      status: 401,
      contentType: "application/json",
      headers: corsHeaders(),
      body: JSON.stringify({ ok: false, error: "unauthorized" }),
    });
  });
}

async function addGuestCookie(page: Page) {
  const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000";
  const url = new URL(baseURL);
  await page.context().addCookies([{
    name: "mindstrata_guest_user_id",
    value: "e2e-fake-guest",
    domain: url.hostname,
    path: "/",
    httpOnly: false,
    secure: url.protocol === "https:",
    sameSite: "Lax",
  }]);
}

test("blog and shop are hidden from main and access when runtime flags are disabled", async ({ page }) => {
  await mockSiteContent(page, {
    "features.blog_enabled": false,
    "features.shop_enabled": false,
    "landing.blog_button_label": "Блог",
    "landing.shop_button_label": "Магазин решений",
    "access.shop_button_label": "Посмотреть готовые решения",
  });
  await mockGuestProfile(page);
  await page.route("**/api/public/demo-modes", async (route: Route) => {
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ok: true, modes: [] }) });
  });

  await gotoApp(page, "/");
  await expect(page.getByRole("link", { name: "Блог" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Магазин решений" })).toHaveCount(0);

  await gotoApp(page, "/access?force=promo");
  await expect(page.getByRole("heading", { name: "Введите промокод" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Блог" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Магазин решений" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Посмотреть готовые решения" })).toHaveCount(0);
});

test("chat shows CMS warning before quota is exhausted", async ({ page }) => {
  await addGuestCookie(page);
  await mockSiteContent(page, {
    "chat.quota.warning_threshold_remaining": 2,
    "chat.quota.warning_title": "Осталось немного ходов",
    "chat.quota.warning_body": "Соберите главный вопрос в одно сообщение. Осталось {{remaining}} из {{limit}}.",
  });
  await page.route("**/api/chat/start", async (route: Route) => {
    if (route.request().method() === "OPTIONS") {
      await route.fulfill({ status: 204, headers: corsHeaders() });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      headers: corsHeaders(),
      body: JSON.stringify({
        ok: true,
        userId: 7,
        role: "user",
        email: "",
        currentModeId: 10,
        currentDialogId: 100,
        modes: [{ id: 10, name: "Психолог", quota: { used: 3, limit: 5, remaining: 2 } }],
        quota: { used: 3, limit: 5, remaining: 2 },
        messages: [{ id: 1, role: "assistant", content: "Готов помочь", createdAt: "2026-06-05T00:00:00Z" }],
      }),
    });
  });
  await page.route("**/api/chat/history?**", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      headers: corsHeaders(),
      body: JSON.stringify({
        ok: true,
        dialogId: 100,
        modeId: 10,
        modeName: "Психолог",
        messages: [],
        quota: { used: 3, limit: 5, remaining: 2 },
        accessActive: true,
      }),
    });
  });

  await gotoApp(page, "/chat");
  await expect(page.getByText("Осталось немного ходов")).toBeVisible();
  await expect(page.getByText("Соберите главный вопрос в одно сообщение. Осталось 2 из 5.")).toBeVisible();
  await expect(page.getByPlaceholder("Напишите сообщение")).toBeVisible();
});
