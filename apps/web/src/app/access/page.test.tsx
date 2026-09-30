import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

const replaceMock = vi.hoisted(() => vi.fn());
const pushMock = vi.hoisted(() => vi.fn());
let search = vi.hoisted(() => new URLSearchParams("force=promo"));
let cmsContent = vi.hoisted(
  () =>
    ({
      "access.promo.help_prefix": "Тестовый доступ выдаёт",
      "access.promo.help_link_href": "https://t.me/qa_bot",
      "access.promo.help_link_label": "QA bot",
      "features.blog_enabled": true,
      "features.shop_enabled": true,
      "landing.blog_button_label": "Блог",
      "landing.shop_button_label": "Магазин решений",
      "access.shop_button_label": "Перейти в магазин решений",
    }) as Record<string, unknown>,
);

vi.mock("@/components/BrandLogo", () => ({
  BrandLogo: () => (
    <a href="/" aria-label="СТРАТУМ Mindstrata">
      logo
    </a>
  ),
}));

vi.mock("@/lib/useSiteContent", () => ({
  useSiteContent: () => cmsContent,
}));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string;
    children: React.ReactNode;
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: replaceMock, push: pushMock }),
  useSearchParams: () => search,
}));

import AccessPage from "./page";

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("AccessPage P0", () => {
  beforeEach(() => {
    replaceMock.mockReset();
    pushMock.mockReset();
    localStorage.clear();
    search = new URLSearchParams("force=promo");
    cmsContent = {
      "access.promo.help_prefix": "Тестовый доступ выдаёт",
      "access.promo.help_link_href": "https://t.me/qa_bot",
      "access.promo.help_link_label": "QA bot",
      "features.blog_enabled": true,
      "features.shop_enabled": true,
      "landing.blog_button_label": "Блог",
      "landing.shop_button_label": "Магазин решений",
      "access.shop_button_label": "Перейти в магазин решений",
    };
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true, hasAccess: false }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    ) as unknown as typeof fetch;
  });

  afterEach(() => {
    cleanup();
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("renders promo form in forced mode without checking existing access", async () => {
    render(<AccessPage />);

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Введите промокод" }),
      ).toBeTruthy(),
    );
    expect(
      screen.getByRole("link", { name: "QA bot" }).getAttribute("href"),
    ).toContain("text=");
    expect(
      screen.getByRole("link", { name: "Блог" }).getAttribute("href"),
    ).toBe("/blog");
    expect(
      screen
        .getByRole("link", { name: "Магазин решений" })
        .getAttribute("href"),
    ).toBe("/shop");
    expect(
      screen
        .getByRole("link", { name: "Перейти в магазин решений" })
        .getAttribute("href"),
    ).toBe("/shop");
    expect(screen.getByRole("button", { name: "Активировать" })).toBeTruthy();
    expect(globalThis.fetch).not.toHaveBeenCalledWith(
      "/api/access/status",
      expect.anything(),
    );
  });

  it("hides blog and shop links on promo access when runtime flags are disabled", async () => {
    cmsContent = {
      "access.promo.help_prefix": "Тестовый доступ выдаёт",
      "access.promo.help_link_href": "https://t.me/qa_bot",
      "access.promo.help_link_label": "QA bot",
      "features.blog_enabled": false,
      "features.shop_enabled": false,
      "landing.blog_button_label": "Блог",
      "landing.shop_button_label": "Магазин решений",
      "access.shop_button_label": "Перейти в магазин решений",
    };

    render(<AccessPage />);

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Введите промокод" }),
      ).toBeTruthy(),
    );
    expect(screen.queryByRole("link", { name: "Блог" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Магазин решений" })).toBeNull();
    expect(
      screen.queryByRole("link", { name: "Перейти в магазин решений" }),
    ).toBeNull();
  });

  it("redirects users with existing access from default /access to /chat", async () => {
    search = new URLSearchParams("");
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true, hasAccess: true }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    ) as unknown as typeof fetch;

    render(<AccessPage />);

    await waitFor(() => expect(replaceMock).toHaveBeenCalledWith("/chat"));
  });

  it("shows empty promo validation error before touching backend", async () => {
    render(<AccessPage />);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Активировать" })).toBeTruthy(),
    );

    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));

    await waitFor(() =>
      expect(screen.getAllByText("Введите промокод").length).toBeGreaterThan(1),
    );
    expect(globalThis.fetch).not.toHaveBeenCalled();
  });

  it("stores pending promocode and sends unauthenticated users through registration", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ok: false,
          requiresAuth: true,
          code: "auth_required",
        }),
        {
          status: 401,
          headers: { "content-type": "application/json" },
        },
      ),
    ) as unknown as typeof fetch;

    render(<AccessPage />);
    await waitFor(() =>
      expect(screen.getByPlaceholderText("XXXX-XXXX-XXXX")).toBeTruthy(),
    );
    fireEvent.change(screen.getByPlaceholderText("XXXX-XXXX-XXXX"), {
      target: { value: "abc-123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));

    await waitFor(() =>
      expect(pushMock).toHaveBeenCalledWith(
        "/register?next=%2Faccess%3Fforce%3Dpromo",
      ),
    );
    expect(localStorage.getItem("mindstrata_pending_promo")).toBe("ABC-123");
  });

  it("prevents duplicate activation submit while request is pending", async () => {
    const activation = createDeferred<Response>();
    globalThis.fetch = vi
      .fn()
      .mockReturnValue(activation.promise) as unknown as typeof fetch;

    render(<AccessPage />);
    const input = await screen.findByPlaceholderText("XXXX-XXXX-XXXX");
    fireEvent.change(input, { target: { value: "dup-123" } });
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));
    fireEvent.click(screen.getByRole("button", { name: /Проверяем/i }));

    expect(globalThis.fetch).toHaveBeenCalledTimes(1);
    activation.resolve(
      new Response(JSON.stringify({ ok: false, code: "invalid_promo" }), {
        status: 400,
        headers: { "content-type": "application/json" },
      }),
    );
    await screen.findByText("Ошибка сервера при проверке промокода");
  });

  it("keeps promo input after backend validation error", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: false, code: "invalid_promo" }), {
        status: 400,
        headers: { "content-type": "application/json" },
      }),
    ) as unknown as typeof fetch;

    render(<AccessPage />);
    const input = (await screen.findByPlaceholderText(
      "XXXX-XXXX-XXXX",
    )) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "bad-code" } });
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));

    await screen.findByText("Ошибка сервера при проверке промокода");
    expect(input.value).toBe("BAD-CODE");
    expect(pushMock).not.toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it("shows max-use exhaustion on the promo form without sending guests to login", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: false, code: "limit_reached" }), {
        status: 400,
        headers: { "content-type": "application/json" },
      }),
    ) as unknown as typeof fetch;

    render(<AccessPage />);
    const input = (await screen.findByPlaceholderText(
      "XXXX-XXXX-XXXX",
    )) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "sold-out" } });
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));

    await screen.findByText("Лимит активаций исчерпан");
    expect(input.value).toBe("SOLD-OUT");
    expect(localStorage.getItem("mindstrata_pending_promo")).toBeNull();
    expect(pushMock).not.toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it("shows network failure without redirecting away from the promo form", async () => {
    globalThis.fetch = vi
      .fn()
      .mockRejectedValue(new Error("network down")) as unknown as typeof fetch;

    render(<AccessPage />);
    const input = await screen.findByPlaceholderText("XXXX-XXXX-XXXX");
    fireEvent.change(input, { target: { value: "net-123" } });
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));

    await waitFor(() =>
      expect(screen.getByText(/Сервер промокодов не отвечает/i)).toBeTruthy(),
    );
    expect(pushMock).not.toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it("keeps mobile forced promo form usable through successful activation", async () => {
    Object.defineProperty(window, "innerWidth", {
      value: 390,
      writable: true,
      configurable: true,
    });
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ok: true,
          code: "MOBILE-CODE",
          accessDays: 7,
          modes: [{ modeId: 20, modeName: "Мобильный режим" }],
          grantedModes: [{ modeId: 20, modeName: "Мобильный режим" }],
          extendedModes: [],
        }),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        },
      ),
    ) as unknown as typeof fetch;

    render(<AccessPage />);
    const input = await screen.findByPlaceholderText("XXXX-XXXX-XXXX");
    expect(screen.getByRole("button", { name: "Активировать" })).toBeTruthy();
    fireEvent.change(input, { target: { value: "mobile-code" } });
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Доступ открыт" }),
      ).toBeTruthy(),
    );
    expect(localStorage.getItem("ms_chat_selected_mode_ids")).toBe("[20]");
  });

  it("stores granted mode ids and builds chat handoff after successful activation", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ok: true,
          code: "P0-CODE",
          accessDays: 14,
          modes: [
            {
              modeId: 10,
              modeName: "Психолог",
              activeTo: "2026-06-15T00:00:00Z",
            },
            {
              modeId: 10,
              modeName: "Психолог",
              activeTo: "2026-06-15T00:00:00Z",
            },
            { modeName: "Без id" },
          ],
          grantedModes: [{ modeId: 10, modeName: "Психолог" }],
          extendedModes: [],
        }),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        },
      ),
    ) as unknown as typeof fetch;

    render(<AccessPage />);
    await waitFor(() =>
      expect(screen.getByPlaceholderText("XXXX-XXXX-XXXX")).toBeTruthy(),
    );
    fireEvent.change(screen.getByPlaceholderText("XXXX-XXXX-XXXX"), {
      target: { value: "p0-code" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Доступ открыт" }),
      ).toBeTruthy(),
    );
    expect(localStorage.getItem("ms_chat_selected_mode_ids")).toBe("[10]");
    expect(
      screen.getByRole("link", { name: /перейти в чат/i }).getAttribute("href"),
    ).toBe("/chat?modes=10");
  });

  it("lets guest promo users enter chat without login after successful activation", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ok: true,
          code: "GUEST-CODE",
          accessDays: 3,
          modes: [{ modeId: 42, modeName: "Гостевой режим" }],
          grantedModes: [{ modeId: 42, modeName: "Гостевой режим" }],
          extendedModes: [],
        }),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        },
      ),
    ) as unknown as typeof fetch;

    render(<AccessPage />);
    const input = await screen.findByPlaceholderText("XXXX-XXXX-XXXX");
    fireEvent.change(input, { target: { value: "guest-code" } });
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Доступ открыт" }),
      ).toBeTruthy(),
    );
    expect(pushMock).not.toHaveBeenCalledWith(
      "/register?next=%2Faccess%3Fforce%3Dpromo",
    );
    expect(localStorage.getItem("mindstrata_pending_promo")).toBeNull();
    expect(localStorage.getItem("ms_chat_selected_mode_ids")).toBe("[42]");

    expect(
      screen.getByRole("link", { name: /перейти в чат/i }).getAttribute("href"),
    ).toBe("/chat?modes=42");
  });
});
