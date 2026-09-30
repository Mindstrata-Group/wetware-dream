import { defineConfig, devices } from "@playwright/test";

// Playwright E2E targets a running deployment (default: local http://localhost:3000;
// CI sets PLAYWRIGHT_BASE_URL explicitly: the staging address).
//
// Run: cd apps/web && npx playwright test
// Run specific:  npx playwright test login.spec
const BASE_URL = process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000";

export default defineConfig({
  testDir: "./e2e",
  timeout: 90_000,
  expect: { timeout: 5_000 },
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 5 : 1,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: BASE_URL,
    ignoreHTTPSErrors: true,
    // The tests are written for the RU version of the site. Without this, the runner's Accept-Language
    // (en-US) triggers the middleware geo-redirect to /en/* and breaks the locators.
    locale: "ru-RU",
    // On the weak stage server Chromium sometimes hangs on HTTP/2 navigation right after a deploy.
    // E2E checks UI behaviour, so we use HTTP/1.1 for a stable transport layer.
    launchOptions: { args: ["--disable-http2"] },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
});
