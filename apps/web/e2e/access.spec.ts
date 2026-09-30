import { test, expect } from "@playwright/test";
import { gotoApp } from "./helpers";

// Public/landing flow: chat start page should redirect to /access (or login)
// when there's no auth — must NOT 500.
test("anonymous user accessing /chat is redirected, not crashed", async ({ page }) => {
  // Middleware does a server-side redirect to /login; we catch the final URL.
  await gotoApp(page, "/chat");
  expect(page.url(), "anonymous /chat must redirect to /login").toContain("/login");
});

// Promocode input page (/access?force=promo) should render the form.
test("promocode input page renders", async ({ page }) => {
  await gotoApp(page, "/access?force=promo");
  // Page is a 'use client' component — hydration and useEffect complete after
  // navigation. Wait for the input to appear before asserting.
  await page.locator("input").first().waitFor({ state: "visible", timeout: 8000 });
  const inputs = await page.locator("input").count();
  expect(inputs).toBeGreaterThan(0);
});
