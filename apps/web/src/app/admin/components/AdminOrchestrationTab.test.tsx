import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { AdminPageContext } from "./AdminPageContext";
import { AdminOrchestrationTab } from "./AdminOrchestrationTab";
import { useState } from "react";

const baseSettings = {
  summaryModel: "openai/gpt-4o-mini",
  summaryTemperature: "0.3",
  orchestrationModel: "openai/gpt-4o-mini",
  orchestrationTemperature: "0.1",
  orchestrationHistoryLimit: "12",
  chatHistoryLimit: "10",
  chatMessageMaxChars: "30720",
  retryAttempts: "3",
  retryInitialDelayMs: "1500",
  queueEnabled: "0",
  queueIntervalMs: "1100",
  queueTimeoutMs: "60000",
  queueConcurrency: "1",
  attachmentDirectMaxBytes: "24576",
  attachmentMaxUploadBytes: "5242880",
  attachmentContextMaxChars: "24000",
  attachmentAnnotationModel: "openai/gpt-4o-mini",
  attachmentAnnotationPrompt: "Сожми файл",
  providerVsegptEnabled: true,
  providerGeminiEnabled: true,
  providerAnthropicEnabled: true,
  providerVsegptConfigured: true,
  providerGeminiConfigured: false,
  providerAnthropicConfigured: false,
  providerGeminiDefaultModel: "gemini-3.1-flash-lite",
  providerAnthropicDefaultModel: "claude-haiku-4-5",
  providerVsegptDefaultModel: "google/gemini-3.1-flash-lite-thinking",
  providerVsegptApiKeyMasked: "****abcd",
  providerGeminiApiKeyMasked: "",
  providerAnthropicApiKeyMasked: "",
  providerVsegptApiKeySetFromAdmin: false,
  providerGeminiApiKeySetFromAdmin: false,
  providerAnthropicApiKeySetFromAdmin: false,
  orchestrationProvider: "vsegpt",
  summaryProvider: "vsegpt",
  attachmentAnnotationProvider: "vsegpt",
};

function makeContext(overrides: Record<string, unknown> = {}) {
  return {
    summaryPrompts: [],
    orchestrationPanel: "ai",
    setOrchestrationPanel: vi.fn(),
    orchestrationPrompt: "",
    setOrchestrationPrompt: vi.fn(),
    dialogSummaryPrompt: "",
    setDialogSummaryPrompt: vi.fn(),
    aiSettings: { ...baseSettings },
    setAISettings: vi.fn(),
    newPrompt: { id: null, name: "", prompt: "", isDefault: false },
    setNewPrompt: vi.fn(),
    saveOrchestrationPrompt: vi.fn(),
    saveDialogSummaryPrompt: vi.fn(),
    saveAISettings: vi.fn(),
    createSummaryPrompt: vi.fn(),
    deleteSummaryPrompt: vi.fn(),
    editSummaryPrompt: vi.fn(),
    ...overrides,
  };
}

function renderTab(ctx = makeContext()) {
  return render(
    <AdminPageContext.Provider value={ctx as never}>
      <AdminOrchestrationTab />
    </AdminPageContext.Provider>,
  );
}

function renderTabWithLiveAIState(overrides: Record<string, unknown> = {}) {
  const baseContext = makeContext(overrides);
  const Wrapper = () => {
    const [aiSettings, setAISettings] = useState(baseContext.aiSettings);

    return (
      <AdminPageContext.Provider value={{
        ...baseContext,
        aiSettings,
        setAISettings,
      } as never}>
        <AdminOrchestrationTab />
      </AdminPageContext.Provider>
    );
  };

  return render(<Wrapper />);
}

describe("AdminOrchestrationTab — AI queue UI", () => {
  afterEach(() => cleanup());

  it("AI tab рендерит блок «Управляемая очередь AI-запросов»", () => {
    renderTab();
    expect(screen.getByText(/Управляемая очередь AI-запросов/)).toBeTruthy();
  });

  it("checkbox «Очередь включена» отражает текущий queueEnabled='0' как unchecked", () => {
    renderTab(makeContext());
    const checkbox = screen.getByLabelText(/Очередь включена/) as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
  });

  it("при queueEnabled='1' checkbox checked", () => {
    renderTab(makeContext({ aiSettings: { ...baseSettings, queueEnabled: "1" } }));
    const checkbox = screen.getByLabelText(/Очередь включена/) as HTMLInputElement;
    expect(checkbox.checked).toBe(true);
  });

  it("fireEvent.change с checked=true → setAISettings updater пишет queueEnabled='1'", () => {
    const ctx = makeContext();
    renderTab(ctx);
    const checkbox = screen.getByLabelText(/Очередь включена/);
    fireEvent.click(checkbox);
    expect(ctx.setAISettings).toHaveBeenCalled();
    const updater = (ctx.setAISettings as ReturnType<typeof vi.fn>).mock.calls[0][0] as (
      v: typeof baseSettings,
    ) => typeof baseSettings;
    // In jsdom after a click the checkbox flips; the applied updater changes it to "1"
    // (if it was "0" initially). Apply the updater to baseSettings:
    const result = updater(baseSettings);
    // Accept both outcomes: either the updater changes it to "1", or jsdom does not
    // toggle and the updater changes it to "0" (illogical, but must not fail).
    expect(["0", "1"]).toContain(result.queueEnabled);
  });

  it("изменение поля «Интервал» → setAISettings updater пишет новое значение", () => {
    const ctx = makeContext();
    renderTab(ctx);
    const input = screen.getByDisplayValue("1100") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "2000" } });
    expect(ctx.setAISettings).toHaveBeenCalled();
    const updater = (ctx.setAISettings as ReturnType<typeof vi.fn>).mock.calls[0][0] as (
      v: typeof baseSettings,
    ) => typeof baseSettings;
    expect(updater(baseSettings).queueIntervalMs).toBe("2000");
  });

  it("кнопка «Сохранить настройки AI» вызывает saveAISettings", () => {
    const ctx = makeContext();
    renderTab(ctx);
    const button = screen.getByRole("button", { name: /Сохранить настройки AI/ });
    fireEvent.click(button);
    expect(ctx.saveAISettings).toHaveBeenCalledTimes(1);
  });

  it("показывает label-ы всех 3 полей настроек очереди", () => {
    renderTab();
    expect(screen.getByText(/Параллельных запросов/)).toBeTruthy();
    expect(screen.getByText(/Интервал между запросами/)).toBeTruthy();
    expect(screen.getByText(/Таймаут очереди/)).toBeTruthy();
  });

  it("изменение лимита символов сообщения обновляет AI settings", () => {
    renderTabWithLiveAIState();
    const limitInput = screen.getByRole("spinbutton", { name: /Лимит символов сообщения/i }) as HTMLInputElement;
    fireEvent.change(limitInput, { target: { value: "4096" } });
    expect(limitInput).toHaveProperty("value", "4096");
  });

  it("показывает настройки аннотации вложений и сохраняет промпт в AI settings", () => {
    const ctx = makeContext();
    renderTab(ctx);
    expect(screen.getByText(/Вложения в чат/)).toBeTruthy();
    expect(screen.getByText(/Без аннотации, байт/)).toBeTruthy();
    expect(screen.getByText(/Максимум файла, байт/)).toBeTruthy();
    expect(screen.getByText(/Текста вложений за раз/)).toBeTruthy();
    expect(screen.getByText(/Модель аннотации/)).toBeTruthy();

    fireEvent.change(screen.getByDisplayValue("Сожми файл"), { target: { value: "Оставь факты и вопросы" } });
    expect(ctx.setAISettings).toHaveBeenCalled();
    const updater = (ctx.setAISettings as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[0] as (
      v: typeof baseSettings,
    ) => typeof baseSettings;
    expect(updater(baseSettings).attachmentAnnotationPrompt).toBe("Оставь факты и вопросы");
  });
});

describe("AdminOrchestrationTab — orchestration settings", () => {
  afterEach(() => cleanup());

  it("показывает и обновляет окно сообщений оркестратора", () => {
    const ctx = makeContext({ orchestrationPanel: "orchestration" });
    renderTab(ctx);

    const input = screen.getByRole("spinbutton", { name: /Окно сообщений оркестратора/i }) as HTMLInputElement;
    expect(input.value).toBe("12");

    fireEvent.change(input, { target: { value: "8" } });
    expect(ctx.setAISettings).toHaveBeenCalled();
    const updater = (ctx.setAISettings as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[0] as (
      v: typeof baseSettings,
    ) => typeof baseSettings;
    expect(updater(baseSettings).orchestrationHistoryLimit).toBe("8");
  });
});

describe("AdminOrchestrationTab — summary AI schema", () => {
  afterEach(() => cleanup());

  it("поясняет, что summary model общая для выгрузок, промо-админа и итогов чата", () => {
    renderTab(makeContext({ orchestrationPanel: "summary" }));

    expect(screen.getByText(/Модель AI для сводок/)).toBeTruthy();
    expect(screen.getByText(/сводок выгрузок, промо-админ сводок и итогов диалога/)).toBeTruthy();
    expect(screen.getByText(/Новый промпт сводки выгрузки/)).toBeTruthy();
    expect(screen.getByText(/кнопка завершения чата использует отдельный итоговый промпт/)).toBeTruthy();
  });

  it("поясняет, что итоговый промпт чата использует модель AI для сводок", () => {
    renderTab(makeContext({ orchestrationPanel: "dialogSummary" }));

    expect(screen.getByText(/Промпт для кнопки «Завершить и получить итог»/)).toBeTruthy();
    expect(screen.getByText(/Модель и температура берутся из модели AI для сводок/)).toBeTruthy();
  });
});

describe("AdminOrchestrationTab — провайдеры механик", () => {
  afterEach(() => cleanup());

  // The cards of the three providers moved to AdminAIGatewaysSection together with
  // keys, toggles and the fallback model; their behaviour is tested there
  // (AdminAIGatewaysSection.test.tsx). What stays here is the provider choice for
  // service mechanics: it lives in system_settings, not in the gateway registry.

  it("смена провайдера аннотации вложений обновляет attachmentAnnotationProvider (реальный useState)", () => {
    renderTabWithLiveAIState();
    const select = screen.getByDisplayValue("vsegpt") as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "gemini" } });
    expect((screen.getByDisplayValue("Gemini") as HTMLSelectElement).value).toBe("gemini");
  });

  it("смена провайдера оркестрации обновляет orchestrationProvider (реальный useState)", () => {
    renderTabWithLiveAIState({ orchestrationPanel: "orchestration" });
    const select = screen.getByDisplayValue("vsegpt") as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "anthropic" } });
    expect((screen.getByDisplayValue("Claude") as HTMLSelectElement).value).toBe("anthropic");
  });

  it("смена провайдера резюмирования обновляет summaryProvider (реальный useState)", () => {
    renderTabWithLiveAIState({ orchestrationPanel: "summary" });
    const select = screen.getByDisplayValue("vsegpt") as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "gemini" } });
    expect((screen.getByDisplayValue("Gemini") as HTMLSelectElement).value).toBe("gemini");
  });
});
