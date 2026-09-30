import { test, expect } from "@playwright/test";
import { gotoApp } from "./helpers";

// Sanity: deployment is reachable and serves the landing page.
// This is the first thing that fails on a botched deploy.
test("landing page loads and shows brand", async ({ page }) => {
  await gotoApp(page, "/");
  await expect(page).toHaveURL(/.*\//);
  // The page title is set in app/layout.tsx (Mindstrata).
  await expect(page).toHaveTitle(/Mindstrata|СТРАТУМ/i);
});

test("/api/health returns ok", async ({ request }) => {
  const res = await request.get("/api/health").catch(() => null);
  // The API mounts /health at root (no /api prefix in some configs).
  // Try both; the deployment-specific path will succeed.
  if (!res || !res.ok()) {
    const alt = await request.get("/health");
    expect(alt.ok(), `health endpoint not reachable`).toBeTruthy();
    const body = await alt.json();
    expect(body.ok).toBe(true);
    return;
  }
  const body = await res.json();
  expect(body.ok).toBe(true);
});
