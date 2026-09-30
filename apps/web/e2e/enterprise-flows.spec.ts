import { expect, test, type Page, type Route } from "@playwright/test";
import { gotoApp } from "./helpers";

function corsHeaders() {
  return {
    "Access-Control-Allow-Origin": new URL(
      process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000",
    ).origin,
    "Access-Control-Allow-Credentials": "true",
    "Access-Control-Allow-Headers": "content-type",
    "Access-Control-Allow-Methods": "POST, OPTIONS",
  };
}

async function addGuestCookie(page: Page) {
  const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000";
  const url = new URL(baseURL);
  await page.context().addCookies([
    {
      name: "mindstrata_guest_user_id",
      value: "e2e-fake-guest",
      domain: url.hostname,
      path: "/",
      httpOnly: false,
      secure: url.protocol === "https:",
      sameSite: "Lax",
    },
  ]);
}

test("guest promo activates access and enters chat without login", async ({
  page,
}) => {
  async function fulfillGuestPromo(route: Route) {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        ok: true,
        code: "GUEST-CODE",
        accessDays: 3,
        modes: [{ modeId: 42, modeName: "Гостевой режим" }],
        grantedModes: [{ modeId: 42, modeName: "Гостевой режим" }],
        extendedModes: [],
      }),
      headers: {
        "Set-Cookie":
          "mindstrata_guest_user_id=e2e-fake-guest; Path=/; SameSite=Lax",
      },
    });
  }
  await page.route("**/api/promo/validate", fulfillGuestPromo);
  await page.route("**/api/access/promocode/apply", fulfillGuestPromo);
  await page.route("**/api/access/status", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      headers: corsHeaders(),
      body: JSON.stringify({ ok: true, hasAccess: false, activeModes: [] }),
    });
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
        role: "guest",
        email: "",
        currentModeId: 42,
        currentDialogId: 4200,
        modes: [
          {
            id: 42,
            name: "Гостевой режим",
            quota: { used: 0, limit: 3, remaining: 3 },
          },
        ],
        quota: { used: 0, limit: 3, remaining: 3 },
        messages: [
          {
            id: 1,
            role: "assistant",
            content: "Гостевой чат открыт",
            createdAt: "2026-06-05T00:00:00Z",
          },
        ],
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
        dialogId: 4200,
        modeId: 42,
        modeName: "Гостевой режим",
        messages: [
          {
            id: 1,
            role: "assistant",
            content: "Гостевой чат открыт",
            createdAt: "2026-06-05T00:00:00Z",
          },
        ],
        accessActive: true,
      }),
    });
  });
  await page.route("**/api/chat/select-mode", async (route: Route) => {
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
        dialogId: 4200,
        modeId: 42,
        modeName: "Гостевой режим",
        messages: [
          {
            id: 1,
            role: "assistant",
            content: "Гостевой чат открыт",
            createdAt: "2026-06-05T00:00:00Z",
          },
        ],
        quota: { used: 0, limit: 3, remaining: 3 },
      }),
    });
  });

  await gotoApp(page, "/access?force=promo");
  await page.getByPlaceholder("XXXX-XXXX-XXXX").fill("guest-code");
  await page.getByRole("button", { name: "Активировать" }).click();

  const chatLink = page.getByRole("link", { name: /перейти в чат/i });
  await expect(chatLink).toBeVisible();
  const chatHref = await chatLink.getAttribute("href");
  expect(chatHref).toBe("/chat?modes=42");
  if (!chatHref) throw new Error("chat handoff link is missing");
  await addGuestCookie(page);
  await gotoApp(page, chatHref);
  await expect(page).toHaveURL(/\/chat(?:\?modes=42)?$/);
  await expect(
    page.getByRole("button", { name: "Гостевой режим" }),
  ).toBeVisible();
  await expect(page.getByPlaceholder("Напишите сообщение")).toBeVisible();
});

test("guest sees login CTA only after quota is exhausted", async ({ page }) => {
  await addGuestCookie(page);
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
        role: "guest",
        email: "",
        currentModeId: 10,
        currentDialogId: 100,
        modes: [
          {
            id: 10,
            name: "Психолог",
            quota: { used: 0, limit: 1, remaining: 1 },
          },
        ],
        quota: { used: 0, limit: 1, remaining: 1 },
        messages: [
          {
            id: 1,
            role: "assistant",
            content: "Готов помочь",
            createdAt: "2026-06-05T00:00:00Z",
          },
        ],
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
        accessActive: true,
      }),
    });
  });
  await page.route("**/api/chat/send", async (route: Route) => {
    if (route.request().method() === "OPTIONS") {
      await route.fulfill({
        status: 204,
        headers: corsHeaders(),
      });
      return;
    }
    await route.fulfill({
      status: 429,
      contentType: "application/json",
      headers: corsHeaders(),
      body: JSON.stringify({
        ok: false,
        error: "daily quota exhausted",
        quota: { used: 1, limit: 1, remaining: 0 },
      }),
    });
  });

  await gotoApp(page, "/chat");
  await expect(page.getByText("Готов помочь")).toBeVisible();
  await page.getByPlaceholder("Напишите сообщение").fill("Последняя попытка");
  await page.getByTitle("Отправить").click();

  await expect(
    page.getByText("Дневной лимит сообщений исчерпан"),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Войти и сохранить историю" }),
  ).toHaveAttribute("href", "/login?next=/chat");
});

test("deleted OAuth account login points user to registration", async ({
  page,
}) => {
  await gotoApp(page, "/login?error=oauth_account");

  await expect(
    page.getByText(/Этот Яндекс ID ещё не зарегистрирован в Стратуме/),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: /Зарегистрироваться с Яндекс ID/i }),
  ).toHaveAttribute("href", "/register?next=%2Fprofile");
});
