import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  act,
  cleanup,
  waitFor,
} from "@testing-library/react";

// Mock apiFetch BEFORE importing the component.
const apiFetchMock = vi.fn();
vi.mock("@/lib/api", () => ({
  apiFetch: (...args: unknown[]) => apiFetchMock(...args),
}));
vi.mock("@/lib/siteContent", async (orig) => {
  const actual = await orig<typeof import("@/lib/siteContent")>();
  return { ...actual, clearSiteContentCache: vi.fn() };
});

import { AdminContentTab } from "./AdminContentTab";

describe("AdminContentTab — auto-save flow", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    apiFetchMock.mockReset();
    // Default: GET /api/admin/site-content -> empty items.
    apiFetchMock.mockImplementation(async (url: string) => {
      if (url === "/api/admin/site-content") return { items: [] };
      return { ok: true };
    });
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("загружает список при mount", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });
    expect(apiFetchMock).toHaveBeenCalledWith("/api/admin/site-content");
  });

  it("рендерит подвкладки Главная/О сервисе/Документы", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });
    expect(screen.getByText("Главная")).toBeTruthy();
    expect(screen.getByText("О сервисе")).toBeTruthy();
    expect(screen.getByText("Документы")).toBeTruthy();
  });

  it("вкладка «Доступ / Промокод» редактирует кнопку магазина", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByText("Доступ / Промокод"));

    expect(screen.getByText("Текст кнопки магазина")).toBeTruthy();
    expect(
      screen.getByPlaceholderText("Посмотреть готовые решения"),
    ).toBeTruthy();
  });

  it("ВВОД в поле триггерит POST через 5 секунд", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });

    // Find the "Home page title" field
    const input = screen.getByPlaceholderText(
      /Стратум сам выберет режим/i,
    ) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Новый заголовок" } });

    // The POST must not arrive right away.
    const postCalls = () =>
      apiFetchMock.mock.calls.filter((c) => c[1]?.method === "POST");
    expect(postCalls()).toHaveLength(0);

    // After 5 s the debounce fires.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    const calls = postCalls();
    expect(calls.length).toBeGreaterThanOrEqual(1);
    const body = JSON.parse(String(calls[0][1].body));
    expect(body).toEqual({
      key: "landing.hero.title",
      value: "Новый заголовок",
    });
  });

  it("подряд 3 правки → 1 POST (дебаунс склеивает)", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });
    const input = screen.getByPlaceholderText(
      /Стратум сам выберет режим/i,
    ) as HTMLInputElement;

    fireEvent.change(input, { target: { value: "a" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    fireEvent.change(input, { target: { value: "ab" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    fireEvent.change(input, { target: { value: "abc" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    const posts = apiFetchMock.mock.calls.filter(
      (c) => c[1]?.method === "POST",
    );
    expect(posts).toHaveLength(1);
    expect(JSON.parse(String(posts[0][1].body)).value).toBe("abc");
  });

  it("статус ✓ сохранено показывается после успеха", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });
    const input = screen.getByPlaceholderText(
      /Стратум сам выберет режим/i,
    ) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "X" } });
    // Debounce 5s, save resolves, the badge switches to 'saved' (shown for 2.5s).
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
      // A few micro-ticks so the Promise.resolve() from onSave gets flushed.
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });
    expect(screen.getByText(/✓ сохранено/i)).toBeTruthy();
  });

  it("статус ✗ <err> при ошибке save", async () => {
    apiFetchMock.mockImplementation(
      async (url: string, opts?: { method?: string }) => {
        if (url === "/api/admin/site-content" && opts?.method === "POST") {
          throw new Error("Network down");
        }
        if (url === "/api/admin/site-content") return { items: [] };
        return { ok: true };
      },
    );
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });
    const input = screen.getByPlaceholderText(
      /Стратум сам выберет режим/i,
    ) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "oops" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });
    expect(screen.getByText(/Network down/)).toBeTruthy();
  });

  it("переключение подвкладки на «О сервисе» показывает новые поля", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });
    // Click the "About" tab; search by role so it does not clash with
    // other texts on the page.
    const tabs = screen.getAllByText("О сервисе");
    fireEvent.click(tabs[0]);
    expect(screen.getByPlaceholderText(/Не просто нейросеть/i)).toBeTruthy();
    // Several places contain "comparison tables"; take the first match.
    expect(screen.getAllByText(/таблицы сравнения/i).length).toBeGreaterThan(0);
  });

  it("кнопка «Сбросить кэш» вызывает POST /flush", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });
    apiFetchMock.mockClear();
    apiFetchMock.mockResolvedValue({ ok: true, flushed: true });

    const flushBtn = screen.getByText(/Принудительно обновить/i);
    await act(async () => {
      fireEvent.click(flushBtn);
      await vi.runAllTimersAsync();
    });

    const flushCalls = apiFetchMock.mock.calls.filter(
      (c) => typeof c[0] === "string" && c[0].includes("/flush"),
    );
    expect(flushCalls.length).toBeGreaterThanOrEqual(1);
    expect(flushCalls[0][1]?.method).toBe("POST");
  });

  it("вкладка «Витрины» управляет флагами и текстами ссылок витрин", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByText("Витрины"));

    expect(screen.getByText("Показывать блог")).toBeTruthy();
    expect(screen.getByText("Показывать магазин решений")).toBeTruthy();
    expect(screen.getByText("Текст кнопки блога на главной")).toBeTruthy();
    expect(screen.getByText("Текст кнопки магазина на главной")).toBeTruthy();

    const blogLabel = screen.getByPlaceholderText("Блог") as HTMLInputElement;
    fireEvent.change(blogLabel, { target: { value: "Журнал" } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });

    const postCall = apiFetchMock.mock.calls.find((c) => {
      if (c[0] !== "/api/admin/site-content" || c[1]?.method !== "POST")
        return false;
      return JSON.parse(String(c[1].body)).key === "landing.blog_button_label";
    });
    expect(postCall).toBeTruthy();
    expect(JSON.parse(String(postCall?.[1].body)).value).toBe("Журнал");
  });

  it("переключатель витрины сохраняется сразу без debounce", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByText("Витрины"));
    apiFetchMock.mockClear();

    await act(async () => {
      fireEvent.click(screen.getAllByRole("switch")[0]);
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });

    const postCalls = apiFetchMock.mock.calls.filter(
      (c) => c[0] === "/api/admin/site-content" && c[1]?.method === "POST",
    );
    expect(postCalls).toHaveLength(1);
    expect(JSON.parse(String(postCalls[0][1].body))).toEqual({
      key: "features.blog_enabled",
      value: true,
    });
  });

  it("вкладка «Витрины» редактирует тексты и вкладки магазина", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByText("Витрины"));

    expect(screen.getByText("Текст кнопки покупки")).toBeTruthy();
    expect(screen.getByText("Текст включённого автопродления")).toBeTruthy();
    expect(screen.getByText("Текст разовой покупки")).toBeTruthy();
    expect(screen.getByText("Пустое состояние магазина")).toBeTruthy();
    expect(screen.getByText("Вкладки магазина")).toBeTruthy();
    expect(screen.getByText("Тарифные группы магазина")).toBeTruthy();
    expect(screen.getByText("Преимущества решений")).toBeTruthy();
    expect(screen.getByText("Пояснения про оплату")).toBeTruthy();
    expect(screen.getByText("Статусы жизненного цикла покупки")).toBeTruthy();
    expect(screen.getByText("Сроки покупки")).toBeTruthy();
    expect(
      screen.getAllByDisplayValue(/Для руководителя|Готовые решения/).length,
    ).toBeGreaterThanOrEqual(2);

    const buyLabel = screen.getByPlaceholderText(
      "Купить решение",
    ) as HTMLInputElement;
    fireEvent.change(buyLabel, { target: { value: "Подключить решение" } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });

    let postCall = apiFetchMock.mock.calls.find((c) => {
      if (c[0] !== "/api/admin/site-content" || c[1]?.method !== "POST")
        return false;
      return JSON.parse(String(c[1].body)).key === "shop.buy_button_label";
    });
    expect(postCall).toBeTruthy();
    expect(JSON.parse(String(postCall?.[1].body)).value).toBe(
      "Подключить решение",
    );

    apiFetchMock.mockClear();
    const tabLabel = screen.getAllByDisplayValue(
      "Для руководителя",
    )[0] as HTMLInputElement;
    fireEvent.change(tabLabel, { target: { value: "Для бизнеса" } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });

    postCall = apiFetchMock.mock.calls.find((c) => {
      if (c[0] !== "/api/admin/site-content" || c[1]?.method !== "POST")
        return false;
      return JSON.parse(String(c[1].body)).key === "shop.tabs";
    });
    expect(postCall).toBeTruthy();
    expect(JSON.parse(String(postCall?.[1].body)).value[0].label).toBe(
      "Для бизнеса",
    );

    apiFetchMock.mockClear();
    const periodLabel = screen.getByDisplayValue("1 месяц") as HTMLInputElement;
    fireEvent.change(periodLabel, { target: { value: "Пробный месяц" } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });

    postCall = apiFetchMock.mock.calls.find((c) => {
      if (c[0] !== "/api/admin/site-content" || c[1]?.method !== "POST")
        return false;
      return JSON.parse(String(c[1].body)).key === "shop.periods";
    });
    expect(postCall).toBeTruthy();
    expect(JSON.parse(String(postCall?.[1].body)).value[0].label).toBe(
      "Пробный месяц",
    );
  });

  it("вкладка «Чат» даёт редактировать предупреждение перед окончанием сообщений", async () => {
    await act(async () => {
      render(<AdminContentTab />);
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByText("Чат"));

    expect(
      screen.getByText("Показывать предупреждение перед окончанием сообщений"),
    ).toBeTruthy();
    expect(screen.getByText("Заголовок предупреждения")).toBeTruthy();
    expect(screen.getByText("Текст предупреждения")).toBeTruthy();
    expect(
      screen.getByPlaceholderText("Сегодня осталось мало сообщений"),
    ).toBeTruthy();
    expect(
      screen.getByPlaceholderText(/Осталось {{remaining}} из {{limit}}/),
    ).toBeTruthy();
  });
});
