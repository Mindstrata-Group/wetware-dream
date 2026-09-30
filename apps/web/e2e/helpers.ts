import type { Page, Response } from "@playwright/test";

export async function gotoApp(page: Page, path: string): Promise<Response | null> {
  const options = { waitUntil: "commit" as const, timeout: 20_000 };
  let lastError: unknown = null;

  for (let attempt = 1; attempt <= 4; attempt += 1) {
    try {
      const response = await page.goto(path, options);
      await page.locator("body").waitFor({ state: "attached", timeout: 15_000 });
      return response;
    } catch (err) {
      lastError = err;
      await page.goto("about:blank", { timeout: 5_000 }).catch(() => {});
      await page.waitForTimeout(attempt * 1_000);
    }
  }

  if (lastError instanceof Error) {
    throw lastError;
  }
  throw new Error(`Navigation to ${path} failed`);
}
