import { expect, type Page, test } from "@playwright/test";
import { gotoApp } from "./helpers";

const tariff = {
  id: 701,
  name: "Решение для спикера",
  description: "Понять зал и собрать тёплый спрос после выступления.",
  tariffType: "regular",
  monthlyPrice: 1900,
  dailyMessageLimit: 80,
  limitType: "shared",
  groupName: "Публичные выступления",
  modeIds: [42],
  modes: [{ id: 42, name: "QR-упражнение" }],
};

async function mockShopApis(page: Page, shopEnabled: boolean) {
  await page.route("**/api/public/site-content", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        content: {
          "features.shop_enabled": shopEnabled,
          "features.blog_enabled": false,
          "landing.shop_button_label": "Магазин решений",
          "shop.hero.title": "Магазин решений",
          "shop.hero.subtitle": "Редактируемый текст витрины",
          "shop.buy_button_label": "Купить решение",
          "shop.payment_hint": "Доступ сразу после оплаты.",
          "shop.periods": [
            { id: "1m", label: "1 месяц", months: 1, enabled: true },
            { id: "3m", label: "3 месяца", months: 3, badge: "удобно", enabled: true },
          ],
          "shop.tabs": [
            { label: "Для спикера", groupName: "Публичные выступления", description: "После QR-кода и выступления", enabled: true },
          ],
          "shop.benefits": [
            { title: "Срез аудитории", body: "Видно, с чем люди пришли.", groupName: "Публичные выступления", enabled: true },
          ],
          "shop.payment_notes": [],
          "shop.lifecycle_statuses": [
            { status: "active", label: "Доступ", description: "Откроется после оплаты", enabled: true },
          ],
        },
      }),
    });
  });

  await page.route("**/api/profile", async (route) => {
    await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ ok: false, error: "auth required" }) });
  });

  await page.route("**/api/public/tariffs", async (route) => {
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ tariffs: [tariff] }) });
  });
}

test("shop flag hides direct shop route content", async ({ page }) => {
  await mockShopApis(page, false);

  await gotoApp(page, "/shop");

  await expect(page.getByRole("heading", { name: "Магазин решений пока закрыт" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Магазин решений" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: tariff.name })).toHaveCount(0);
});

test("shop renders editable CMS copy and sends selected period to checkout", async ({ page }) => {
  await mockShopApis(page, true);
  let checkoutPayload: unknown = null;
  await page.route("**/api/payments/yookassa/create", async (route) => {
    checkoutPayload = route.request().postDataJSON();
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ confirmationUrl: "" }) });
  });

  await gotoApp(page, "/shop");

  await expect(page.getByRole("heading", { name: "Магазин решений" })).toBeVisible();
  await expect(page.getByText("Редактируемый текст витрины")).toBeVisible();
  await expect(page.getByRole("tab", { name: "Для спикера" })).toBeVisible();
  await expect(page.getByText("Срез аудитории")).toBeVisible();
  await expect(page.getByText("Доступ сразу после оплаты.")).toBeVisible();
  await expect(page.getByText("Через YooKassa")).toHaveCount(0);

  await page.getByRole("button", { name: /3 месяца/ }).click();
  await page.getByRole("button", { name: "Купить решение" }).click();

  await expect.poll(() => checkoutPayload).toMatchObject({
    tariffId: 701,
    subscriptionMonths: 3,
    enableAutoRenew: true,
  });
});
