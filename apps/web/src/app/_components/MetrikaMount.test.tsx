import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";

import { MetrikaMount } from "./MetrikaMount";

let pathname = "/";
vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
}));

function jsonResponse(body: Record<string, unknown>) {
  return { ok: true, status: 200, json: async () => body } as Response;
}

describe("MetrikaMount", () => {
  beforeEach(() => {
    pathname = "/";
  });

  afterEach(() => {
    cleanup();
    delete (window as { ym?: unknown }).ym;
    document
      .querySelectorAll('script[src*="mc.yandex.ru"]')
      .forEach((el) => el.remove());
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("инициализирует счётчик с номером и параметрами из настроек", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({ ok: true, metrikaCounterId: "98765432", metrikaParams: { webvisor: true } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<MetrikaMount />);

    await waitFor(() => {
      expect(window.ym).toBeTruthy();
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/public/analytics");
    // Before tag.js loads, calls pile up in the ym.a queue; init must be there.
    const queue = (window.ym as unknown as { a: unknown[][] }).a;
    expect(queue).toContainEqual([98765432, "init", { webvisor: true }]);
    expect(document.querySelector('script[src*="mc.yandex.ru/metrika/tag.js"]')).toBeTruthy();
  });

  it("не подключает счётчик, когда номер пустой (аналитика выключена)", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({ ok: true, metrikaCounterId: "", metrikaParams: {} }),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<MetrikaMount />);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled();
    });
    expect(window.ym).toBeUndefined();
    expect(document.querySelector('script[src*="mc.yandex.ru"]')).toBeNull();
  });

  it("не считает служебные разделы (/admin)", async () => {
    pathname = "/admin";
    const fetchMock = vi.fn(async () =>
      jsonResponse({ ok: true, metrikaCounterId: "98765432", metrikaParams: {} }),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<MetrikaMount />);

    // The effect runs synchronously after mount; fetch must not be called.
    await new Promise((r) => setTimeout(r, 20));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(window.ym).toBeUndefined();
  });
});
