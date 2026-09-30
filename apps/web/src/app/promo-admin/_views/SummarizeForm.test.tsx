import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { SummarizeForm } from "./SummarizeForm";
import type { PromoStatus } from "../_types";

const basePromo: PromoStatus = {
  id: 1,
  code: "PROMO1",
  summaryUsed: 0,
  summaryRemaining: 5,
  messageCount: 5,
  activationsWithoutMessages: 0,
};

function renderForm(opts: { status: PromoStatus; promptId?: number | null }) {
  return render(
    <SummarizeForm
      modes={[{ id: 1, name: "Mode A" }]}
      modeIds={[]}
      setModeIds={vi.fn()}
      prompts={[{ id: 7, name: "default", isDefault: true }]}
      promptId={opts.promptId ?? 7}
      setPromptId={vi.fn()}
      summarize={vi.fn()}
      loadingSummary={false}
      status={opts.status}
      copyAccessUrl={vi.fn()}
      mobile={false}
      result=""
      history={[]}
      downloadResult={vi.fn()}
    />,
  );
}

describe("SummarizeForm — guard при отсутствии сообщений", () => {
  afterEach(() => cleanup());

  it("при messageCount>0 кнопка резюмирования активна", () => {
    renderForm({ status: basePromo });
    const button = screen.getByRole("button", { name: /Начать резюмирование/ });
    expect((button as HTMLButtonElement).disabled).toBe(false);
  });

  it("при messageCount=0 кнопка резюмирования disabled", () => {
    const status: PromoStatus = { ...basePromo, messageCount: 0 };
    renderForm({ status });
    const button = screen.getByRole("button", { name: /Начать резюмирование/ });
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });

  it("при messageCount=0 показывается warning hint о пустой выборке", () => {
    const status: PromoStatus = {
      ...basePromo,
      messageCount: 0,
      activationsWithoutMessages: 3,
    };
    renderForm({ status });
    expect(screen.getByText(/пока нечего резюмировать/i)).toBeTruthy();
  });

  it("при promptId=0 (falsy) кнопка disabled даже при сообщениях", () => {
    // null is no longer a valid prompt, but the component renders because prompts.length>0.
    // Use 0 as an explicitly falsy ID, which !promptId turns into disabled.
    renderForm({ status: basePromo, promptId: 0 });
    const button = screen.getByRole("button", { name: /Начать резюмирование/ });
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });
});
