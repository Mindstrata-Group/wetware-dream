import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { AdminAIGatewaysSection } from "./AdminAIGatewaysSection";

const apiFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api", () => ({ apiFetch: apiFetchMock }));

const gateways = [
  {
    id: "anthropic",
    title: "Claude (Anthropic)",
    protocol: "anthropic",
    baseUrl: "",
    relayUrl: "",
    useRelay: false,
    effectiveUrl: "",
    apiKeyMasked: "****abcd",
    apiKeySource: "legacy",
    defaultModel: "claude-haiku-4-5",
    priority: 10,
    enabled: true,
    archived: false,
    configured: true,
  },
  {
    id: "relay-fi",
    title: "Зарубежный релей",
    protocol: "openai",
    baseUrl: "https://relay.example/v1",
    relayUrl: "https://vps.example/secret/v1",
    useRelay: false,
    effectiveUrl: "https://relay.example/v1",
    apiKeyMasked: "****9999",
    apiKeySource: "gateway",
    defaultModel: "openai/gpt-5",
    priority: 20,
    enabled: true,
    archived: false,
    configured: true,
  },
  {
    id: "old-relay",
    title: "Старый релей",
    protocol: "openai",
    baseUrl: "",
    relayUrl: "",
    useRelay: false,
    effectiveUrl: "",
    apiKeyMasked: "",
    apiKeySource: "none",
    defaultModel: "",
    priority: 30,
    enabled: false,
    archived: true,
    configured: false,
  },
];

beforeEach(() => {
  apiFetchMock.mockReset();
  apiFetchMock.mockResolvedValue({ ok: true, gateways });
});

afterEach(() => cleanup());

describe("AdminAIGatewaysSection", () => {
  it("показывает шлюзы в порядке фолбека и отдельно архив", async () => {
    render(<AdminAIGatewaysSection />);

    await screen.findByText("Claude (Anthropic)");
    expect(screen.getByText("Зарубежный релей")).toBeTruthy();
    // Archived ones get their own section instead of being mixed with active ones.
    expect(screen.getByText("Архив")).toBeTruthy();
    expect(screen.getByText("Старый релей")).toBeTruthy();

    // The list order is the fallback order, so positions are numbered.
    expect(screen.getByText("1.")).toBeTruthy();
    expect(screen.getByText("2.")).toBeTruthy();
  });

  it("не даёт поднять верхний шлюз и опустить нижний", async () => {
    render(<AdminAIGatewaysSection />);
    await screen.findByText("Claude (Anthropic)");

    const up = screen.getAllByRole("button", { name: "↑" });
    const down = screen.getAllByRole("button", { name: "↓" });
    expect((up[0] as HTMLButtonElement).disabled).toBe(true);
    expect((down[down.length - 1] as HTMLButtonElement).disabled).toBe(true);
    // Buttons inside the list are live, otherwise there is no way to change the order.
    expect((up[1] as HTMLButtonElement).disabled).toBe(false);
  });

  it("подъём отправляет move up именно для своего шлюза", async () => {
    render(<AdminAIGatewaysSection />);
    await screen.findByText("Зарубежный релей");

    apiFetchMock.mockClear();
    fireEvent.click(screen.getAllByRole("button", { name: "↑" })[1]);

    await waitFor(() => expect(apiFetchMock).toHaveBeenCalled());
    const [path, init] = apiFetchMock.mock.calls[0];
    expect(path).toBe("/api/admin/ai-gateways/relay-fi");
    expect(JSON.parse(init.body)).toEqual({ action: "move", direction: "up" });
  });

  it("показывает источник ключа, а не сам ключ", async () => {
    render(<AdminAIGatewaysSection />);
    await screen.findByText("Claude (Anthropic)");

    // A key inherited from env must be labelled as such, otherwise the admin will not
    // understand why the gateway stops working after a server move.
    expect(screen.getByText(/ключ унаследован из env сервера/)).toBeTruthy();
    expect(screen.getByText(/ключ задан здесь/)).toBeTruthy();
  });

  it("создаёт шлюз с выбранным протоколом", async () => {
    render(<AdminAIGatewaysSection />);
    await screen.findByText("Claude (Anthropic)");

    fireEvent.change(screen.getByPlaceholderText(/^id /), { target: { value: "relay-2" } });
    fireEvent.change(screen.getByPlaceholderText("название для админки"), { target: { value: "Второй релей" } });

    apiFetchMock.mockClear();
    apiFetchMock.mockResolvedValue({ ok: true, gateways });
    fireEvent.click(screen.getByRole("button", { name: "Добавить" }));

    await waitFor(() => expect(apiFetchMock).toHaveBeenCalled());
    const [path, init] = apiFetchMock.mock.calls[0];
    expect(path).toBe("/api/admin/ai-gateways");
    expect(JSON.parse(init.body)).toMatchObject({ id: "relay-2", title: "Второй релей", protocol: "openai" });
  });

  it("галочка «Через ВПС» шлёт useRelay именно для своего шлюза", async () => {
    render(<AdminAIGatewaysSection />);
    await screen.findByText("Зарубежный релей");

    apiFetchMock.mockClear();
    apiFetchMock.mockResolvedValue({ ok: true, gateways });
    // The second gateway in the list is relay-fi; it has an address via the VPS.
    fireEvent.click(screen.getAllByLabelText("Через ВПС")[1]);

    await waitFor(() => expect(apiFetchMock).toHaveBeenCalled());
    const [path, init] = apiFetchMock.mock.calls[0];
    expect(path).toBe("/api/admin/ai-gateways/relay-fi");
    expect(JSON.parse(init.body)).toEqual({ action: "update", useRelay: true });
  });

  it("показывает итоговый адрес — куда реально уходит запрос", async () => {
    render(<AdminAIGatewaysSection />);
    await screen.findByText("Зарубежный релей");
    // Three fields and a checkbox are three places to make a mistake; the admin has one question.
    expect(screen.getByText("https://relay.example/v1")).toBeTruthy();
  });

  it("показывает ошибку сервера вместо тихого отказа", async () => {
    render(<AdminAIGatewaysSection />);
    await screen.findByText("Claude (Anthropic)");

    apiFetchMock.mockRejectedValueOnce({ payload: { error: "это последний включённый шлюз" } });
    fireEvent.click(screen.getAllByRole("button", { name: "В архив" })[0]);

    expect(await screen.findByText("это последний включённый шлюз")).toBeTruthy();
  });
});
