import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/BrandLogo", () => ({
  BrandLogo: () => (
    <a href="/" aria-label="СТРАТУМ Mindstrata">
      logo
    </a>
  ),
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

const loadPublicTariffs = vi.hoisted(() => vi.fn());
const apiFetch = vi.hoisted(() => vi.fn());
vi.mock("@/lib/publicTariffs", async () => {
  const actual = await vi.importActual<typeof import("@/lib/publicTariffs")>(
    "@/lib/publicTariffs",
  );
  return { ...actual, loadPublicTariffs };
});
vi.mock("@/lib/api", () => ({ apiFetch }));

let content = vi.hoisted(
  (): Record<string, unknown> => ({
    "features.shop_enabled": true,
    "shop.hero.title": "Магазин решений",
    "shop.buy_button_label": "Оформить подписку",
  }),
);

vi.mock("@/lib/useSiteContent", () => ({
  useSiteContent: () => content,
}));

import ShopPage from "./page";

describe("ShopPage", () => {
  beforeEach(() => {
    apiFetch.mockImplementation((url: string) => {
      if (url === "/api/profile") return Promise.resolve({});
      return Promise.resolve({});
    });
  });

  afterEach(() => {
    cleanup();
    loadPublicTariffs.mockReset();
    apiFetch.mockReset();
    content = {
      "features.shop_enabled": true,
      "shop.hero.title": "Магазин решений",
      "shop.buy_button_label": "Оформить подписку",
    };
  });

  it("renders tariffs from public API when shop flag is enabled", async () => {
    loadPublicTariffs.mockResolvedValue([
      {
        id: 1,
        name: "Профи",
        description: "Для регулярной работы",
        tariffType: "regular",
        monthlyPrice: 1880,
        dailyMessageLimit: 100,
        limitType: "shared",
        groupName: "Основные",
        modeIds: [6],
        modes: [{ id: 6, name: "Метамодель", welcomeMessage: "Разберём вопрос по метамодели" }],
      },
    ]);

    render(<ShopPage />);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Профи" })).toBeTruthy(),
    );
    expect(screen.getByText("1 880 ₽")).toBeTruthy();
    expect(screen.getByText("До 100 сообщений в день")).toBeTruthy();
    expect(screen.getByText("Метамодель").closest("span")?.getAttribute("aria-label")).toBe("Метамодель: Разберём вопрос по метамодели");
    expect(screen.getByText("Доступ сразу после оплаты.")).toBeTruthy();
    expect(screen.queryByText(/Платёж проходит через YooKassa/i)).toBeNull();
    expect(screen.queryByText(/^Оплата$/i)).toBeNull();
    expect(screen.queryByText(/рекомендуем начать здесь/i)).toBeNull();
  });

  it("creates a YooKassa autorenew payment from a tariff card", async () => {
    loadPublicTariffs.mockResolvedValue([
      {
        id: 7,
        name: "Команда",
        description: "",
        tariffType: "regular",
        monthlyPrice: 2500,
        dailyMessageLimit: 80,
        limitType: "shared",
        groupName: "Бизнес",
        modeIds: [11],
        modes: [{ id: 11, name: "Продажи" }],
      },
    ]);
    apiFetch.mockResolvedValue({});

    render(<ShopPage />);

    const button = await screen.findByRole("button", {
      name: "Оформить подписку",
    });
    fireEvent.click(button);

    await waitFor(() =>
      expect(apiFetch).toHaveBeenCalledWith("/api/payments/yookassa/create", {
        method: "POST",
        body: JSON.stringify({
          tariffId: 7,
          subscriptionMonths: 1,
          enableAutoRenew: true,
          returnUrl: `${window.location.origin}/profile`,
        }),
      }),
    );
  });

  it("creates a one-time payment when autorenew is unchecked", async () => {
    loadPublicTariffs.mockResolvedValue([
      {
        id: 8,
        name: "Разово",
        description: "",
        tariffType: "regular",
        monthlyPrice: 900,
        dailyMessageLimit: 40,
        limitType: "shared",
        groupName: "Основные",
        modeIds: [2],
        modes: [],
      },
    ]);
    apiFetch.mockResolvedValue({});
    content = {
      ...content,
      "shop.autorenew_checkbox_label": "Продлевать каждый месяц",
      "shop.one_time_checkbox_label": "Оплатить один период",
    };

    render(<ShopPage />);

    const checkbox = await screen.findByRole("checkbox", {
      name: /продлевать каждый месяц/i,
    });
    fireEvent.click(checkbox);
    expect(screen.getByText("Оплатить один период")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Оформить подписку" }));

    await waitFor(() =>
      expect(apiFetch).toHaveBeenCalledWith(
        "/api/payments/yookassa/create",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            tariffId: 8,
            subscriptionMonths: 1,
            enableAutoRenew: false,
            returnUrl: `${window.location.origin}/profile`,
          }),
        }),
      ),
    );
  });

  it("shows closed state when shop flag is disabled", async () => {
    content = {
      "features.shop_enabled": false,
      "shop.hero.title": "Магазин решений",
    };
    loadPublicTariffs.mockResolvedValue([]);

    render(<ShopPage />);

    expect(
      screen.getByRole("heading", { name: "Магазин решений пока закрыт" }),
    ).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Магазин решений" })).toBeNull();
    await waitFor(() => expect(apiFetch).toHaveBeenCalledWith("/api/profile"));
  });

  it("uses editable tabs and hides disabled shop sections", async () => {
    content = {
      "features.shop_enabled": true,
      "shop.hero.title": "Магазин решений",
      "shop.loading_text": "Подбираем тарифы",
      "shop.empty_state": "В этой секции пока пусто",
      "shop.buy_button_label": "Подключить решение",
      "shop.benefits": [
        {
          title: "Для команды",
          body: "Продление включим автоматически.",
          groupName: "Бизнес",
          enabled: true,
        },
      ],
      "shop.tabs": [
        {
          label: "Для бизнеса",
          groupName: "Бизнес",
          badge: "проверить спрос",
          description: "Рабочие тарифы",
          enabled: true,
        },
        {
          label: "Скрытый раздел",
          groupName: "Архив",
          description: "Не показывать",
          enabled: false,
        },
      ],
    };
    loadPublicTariffs.mockResolvedValue([
      {
        id: 2,
        name: "Бизнес-пакет",
        description: "",
        tariffType: "regular",
        monthlyPrice: 3900,
        dailyMessageLimit: 150,
        limitType: "shared",
        groupName: "Бизнес",
        modeIds: [3],
        modes: [],
      },
      {
        id: 3,
        name: "Архивный",
        description: "",
        tariffType: "regular",
        monthlyPrice: 100,
        dailyMessageLimit: 10,
        limitType: "shared",
        groupName: "Архив",
        modeIds: [4],
        modes: [],
      },
    ]);

    render(<ShopPage />);

    expect(screen.getByText("Подбираем тарифы")).toBeTruthy();
    await waitFor(() =>
      expect(screen.getByRole("tab", { name: "Для бизнеса" })).toBeTruthy(),
    );
    expect(screen.queryByRole("tab", { name: "Скрытый раздел" })).toBeNull();
    expect(screen.getByText("проверить спрос")).toBeTruthy();
    expect(screen.getByText("Рабочие тарифы")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Бизнес-пакет" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Архивный" })).toBeNull();
    expect(screen.getByText(/Продление включим автоматически/)).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Подключить решение" }),
    ).toBeTruthy();
  });

  it("uses editable purchase periods in the checkout contract", async () => {
    content = {
      "features.shop_enabled": true,
      "shop.hero.title": "Магазин решений",
      "shop.buy_button_label": "Купить решение",
      "shop.periods": [
        { id: "1m", label: "1 месяц", months: 1, enabled: true },
        {
          id: "3m",
          label: "3 месяца",
          months: 3,
          discountPercent: 7,
          badge: "удобно",
          enabled: true,
        },
      ],
    };
    loadPublicTariffs.mockResolvedValue([
      {
        id: 9,
        name: "Решение для спикера",
        description: "",
        tariffType: "regular",
        monthlyPrice: 1000,
        dailyMessageLimit: 30,
        limitType: "shared",
        groupName: "Публичные выступления",
        modeIds: [1],
        modes: [{ id: 1, name: "Спикер", welcomeMessage: "Собрать запросы зала" }],
      },
    ]);
    apiFetch.mockResolvedValue({});

    render(<ShopPage />);

    fireEvent.click(await screen.findByRole("button", { name: /3 месяца/i }));
    fireEvent.click(screen.getByRole("button", { name: "Купить решение" }));

    await waitFor(() =>
      expect(apiFetch).toHaveBeenCalledWith("/api/payments/yookassa/create", {
        method: "POST",
        body: JSON.stringify({
          tariffId: 9,
          subscriptionMonths: 3,
          enableAutoRenew: true,
          returnUrl: `${window.location.origin}/profile`,
        }),
      }),
    );
    expect(screen.getByText(/Итого за 3 мес.: 2 790 ₽/)).toBeTruthy();
    expect(screen.getByText(/скидка 7%/)).toBeTruthy();
    expect(screen.getByText(/Продлевать автоматически: каждые 3 мес/i)).toBeTruthy();
    expect(screen.getByRole("tooltip").textContent).toContain("Собрать запросы зала");
  });
});
