import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { useState } from "react";
import { AdminPageContext, type AdminPageContextValue } from "../AdminPageContext";
import { EditPanel } from "./EditPanel";

const baseModeDetail = {
  id: 1,
  name: "Снизить тревогу",
  hidden: false,
  aiModel: "google/gemini-3.1-flash-lite-thinking",
  aiProvider: "vsegpt" as const,
  thinkingMode: "default" as const,
  modelTemperature: 0.6,
  prompt: "system prompt",
  welcomeMessage: "welcome",
  demoChat: "[]",
  audioEnabled: false,
  criteria: "",
  orchestratorCheckInterval: 5,
  reminderCount: null,
};

function makeContext(overrides: Record<string, unknown> = {}) {
  return {
    modes: [],
    modeMeta: { offset: 0, limit: 50, total: 0 },
    modeDetail: baseModeDetail,
    setModeDetail: vi.fn(),
    modeQ: "",
    setModeQ: vi.fn(),
    newMode: { name: "", prompt: "", welcomeMessage: "", aiModel: "openai/gpt-4o-mini", modelTemperature: "0.7", criteria: "", orchestratorCheckInterval: "5", reminderCount: "0", hidden: false, audioEnabled: false },
    setNewMode: vi.fn(),
    guardrail: "",
    setGuardrail: vi.fn(),
    modelStats: {},
    modePage: 1,
    modeTotalPages: 1,
    loadModes: vi.fn(),
    loadModelStats: vi.fn(),
    openMode: vi.fn(),
    saveMode: vi.fn((e: React.FormEvent) => e.preventDefault()),
    copyMode: vi.fn(),
    createMode: vi.fn(),
    testModeModel: vi.fn(),
    detachModeFromPaidTariffs: vi.fn(),
    cancelDeleteModeHold: vi.fn(),
    startDeleteModeHold: vi.fn(),
    applyGuardrail: vi.fn(),
    testGuardrailModel: vi.fn(),
    pageSizes: [10, 50, 100],
    copy: vi.fn(),
    showNotice: vi.fn(),
    handleError: vi.fn(),
    ...overrides,
  };
}

function renderEditPanel(ctx = makeContext()) {
  return render(
    <AdminPageContext.Provider value={ctx as unknown as AdminPageContextValue}>
      <EditPanel />
    </AdminPageContext.Provider>,
  );
}

// Stateful wrapper (like renderTabWithLiveAIState in AdminOrchestrationTab.test.tsx):
// setModeDetail is a real useState, not a mock, otherwise React reverts the DOM value of the
// controlled <select> back to the prop on the next render, and the
// change event is "lost" for a check via re-render.
function renderEditPanelWithLiveState(overrides: Record<string, unknown> = {}) {
  const base = makeContext(overrides);
  function Wrapper() {
    const [modeDetail, setModeDetail] = useState(base.modeDetail);
    return (
      <AdminPageContext.Provider value={{ ...base, modeDetail, setModeDetail } as unknown as AdminPageContextValue}>
        <EditPanel />
      </AdminPageContext.Provider>
    );
  }
  return render(<Wrapper />);
}

function modelOptionTexts() {
  const select = screen.getByLabelText("Модель AI") as HTMLSelectElement;
  return [...select.querySelectorAll("option")]
    .map((o) => o.getAttribute("value"))
    .filter((v) => v !== "__manual__");
}

describe("EditPanel — провайдер/размышления режима", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("рендерит дропдаун провайдера с текущим значением и меняет его на anthropic", () => {
    renderEditPanelWithLiveState();
    const providerSelect = screen.getByDisplayValue("vsegpt (агрегатор, страховка)") as HTMLSelectElement;
    expect(providerSelect).toBeTruthy();

    fireEvent.change(providerSelect, { target: { value: "anthropic" } });

    expect(screen.getByDisplayValue("Claude напрямую")).toBeTruthy();
  });

  it("рендерит дропдаун размышлений и меняет его на «Максимум»", () => {
    renderEditPanelWithLiveState();
    const thinkingSelect = screen.getByDisplayValue("По умолчанию") as HTMLSelectElement;

    fireEvent.change(thinkingSelect, { target: { value: "high" } });

    expect(screen.getByDisplayValue("Максимум")).toBeTruthy();
  });

  it("список моделей — выпадающий select для провайдера gemini", () => {
    renderEditPanel(makeContext({ modeDetail: { ...baseModeDetail, aiProvider: "gemini", aiModel: "gemini-3.1-flash-lite" } }));
    expect(modelOptionTexts()).toEqual(["gemini-3.5-flash-minimal", "gemini-3.1-flash-lite", "gemini-3.1-pro"]);
  });

  it("список моделей — выпадающий select для провайдера anthropic", () => {
    renderEditPanel(makeContext({ modeDetail: { ...baseModeDetail, aiProvider: "anthropic", aiModel: "claude-sonnet-5" } }));
    expect(modelOptionTexts()).toEqual(["claude-haiku-4-5", "claude-sonnet-5", "claude-opus-4-8"]);
  });

  it("модель — обычное текстовое поле для vsegpt (нет живого списка)", () => {
    renderEditPanel();
    const field = screen.getByLabelText("Модель AI") as HTMLInputElement;
    expect(field.tagName).toBe("INPUT");
    expect(field.value).toBe("google/gemini-3.1-flash-lite-thinking");
  });

  it("текущая модель не из списка попадает отдельным пунктом в select", () => {
    renderEditPanel(makeContext({ modeDetail: { ...baseModeDetail, aiProvider: "gemini", aiModel: "gemini-custom-preview" } }));
    const select = screen.getByLabelText("Модель AI") as HTMLSelectElement;
    expect(select.value).toBe("gemini-custom-preview");
    expect([...select.querySelectorAll("option")].map((o) => o.getAttribute("value"))).toContain("gemini-custom-preview");
  });
});

describe("EditPanel — lead-уведомления режима", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("поля получателей и порога скрыты, пока триггер выключен", () => {
    renderEditPanel();
    expect(screen.getByLabelText(/Уведомлять менеджера о тёплом лиде/)).toBeTruthy();
    expect(screen.queryByLabelText(/Получатели \(Max/)).toBeNull();
    expect(screen.queryByLabelText(/Получатели Telegram/)).toBeNull();
    expect(screen.queryByLabelText(/Слать после N-го ответа/)).toBeNull();
  });

  it("включение триггера показывает получателей (Max и Telegram) и порог с текущими значениями", () => {
    renderEditPanelWithLiveState({
      modeDetail: { ...baseModeDetail, leadNotifyEnabled: false, leadNotifyChatIds: "111222333", leadNotifyTelegramIds: "555666777", leadNotifyThreshold: 4 },
    });
    fireEvent.click(screen.getByLabelText(/Уведомлять менеджера о тёплом лиде/));
    const maxRecipients = screen.getByLabelText(/Получатели \(Max/) as HTMLInputElement;
    const tgRecipients = screen.getByLabelText(/Получатели Telegram/) as HTMLInputElement;
    const threshold = screen.getByLabelText(/Слать после N-го ответа/) as HTMLInputElement;
    expect(maxRecipients.value).toBe("111222333");
    expect(tgRecipients.value).toBe("555666777");
    expect(threshold.value).toBe("4");
  });

  it("правка получателей Max/Telegram и порога обновляет modeDetail", () => {
    renderEditPanelWithLiveState({
      modeDetail: { ...baseModeDetail, leadNotifyEnabled: true, leadNotifyChatIds: "", leadNotifyTelegramIds: "", leadNotifyThreshold: 3 },
    });
    const maxRecipients = screen.getByLabelText(/Получатели \(Max/) as HTMLInputElement;
    fireEvent.change(maxRecipients, { target: { value: "111, 222" } });
    expect((screen.getByLabelText(/Получатели \(Max/) as HTMLInputElement).value).toBe("111, 222");
    const tgRecipients = screen.getByLabelText(/Получатели Telegram/) as HTMLInputElement;
    fireEvent.change(tgRecipients, { target: { value: "@channel" } });
    expect((screen.getByLabelText(/Получатели Telegram/) as HTMLInputElement).value).toBe("@channel");
    const threshold = screen.getByLabelText(/Слать после N-го ответа/) as HTMLInputElement;
    fireEvent.change(threshold, { target: { value: "5" } });
    expect((screen.getByLabelText(/Слать после N-го ответа/) as HTMLInputElement).value).toBe("5");
  });
});
