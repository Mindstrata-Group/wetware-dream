import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act, cleanup, waitFor } from "@testing-library/react";

// Mock apiFetch BEFORE importing the component.
const apiFetchMock = vi.fn();
vi.mock("@/lib/api", () => ({
  apiFetch: (...args: unknown[]) => apiFetchMock(...args),
}));

import { ModeHistoryPanel } from "./ModeHistoryPanel";

const versionsResponse = {
  versions: [
    { sha: "sha-2", authorName: "Second Admin", authorEmail: "second@test.local", createdAt: "2026-07-02T10:00:00Z", message: "режим 1: правка" },
    { sha: "sha-1", authorName: "First Admin", authorEmail: "first@test.local", createdAt: "2026-07-01T10:00:00Z", message: "режим 1: создание" },
  ],
};

const snapshot1 = { name: "Режим", prompt: "Старый промпт", welcomeMessage: "Привет", criteria: "крит" };

describe("ModeHistoryPanel", () => {
  beforeEach(() => {
    apiFetchMock.mockReset();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("грузит список версий при монтировании и не использует git-терминологию", async () => {
    apiFetchMock.mockResolvedValueOnce(versionsResponse);
    await act(async () => {
      render(
        <ModeHistoryPanel
          modeId={1}
          current={{ prompt: "Новый промпт", welcomeMessage: "Привет", criteria: "крит" }}
          onRestored={vi.fn()}
        />,
      );
    });
    expect(apiFetchMock).toHaveBeenCalledWith("/api/admin/modes/1/history");
    await waitFor(() => {
      expect(screen.getAllByText(/Сохранено/).length).toBe(2);
    });
    // No "git"/"commit"/"SHA" in the visible text.
    expect(screen.queryByText(/git/i)).toBeNull();
    expect(screen.queryByText(/commit/i)).toBeNull();
    expect(screen.queryByText(/SHA/)).toBeNull();
  });

  it("пустая история показывает нейтральное сообщение (без ошибок)", async () => {
    apiFetchMock.mockResolvedValueOnce({ versions: [] });
    await act(async () => {
      render(<ModeHistoryPanel modeId={1} current={{ prompt: "x" }} onRestored={vi.fn()} />);
    });
    await waitFor(() => {
      expect(screen.getByText(/Пока нет сохранённых версий/)).toBeTruthy();
    });
  });

  it("клик по версии грузит снапшот и показывает diff промпта", async () => {
    apiFetchMock.mockImplementation(async (url: string) => {
      if (url === "/api/admin/modes/1/history") return versionsResponse;
      if (url === "/api/admin/modes/1/history/sha-1") return { version: snapshot1 };
      throw new Error("unexpected url " + url);
    });
    await act(async () => {
      render(
        <ModeHistoryPanel
          modeId={1}
          current={{ prompt: "Новый промпт", welcomeMessage: "Привет", criteria: "крит" }}
          onRestored={vi.fn()}
        />,
      );
    });
    await waitFor(() => expect(screen.getAllByText(/Сохранено/).length).toBe(2));

    const buttons = screen.getAllByRole("button");
    const olderVersionButton = buttons.find((b) => b.textContent?.includes("First Admin"));
    expect(olderVersionButton).toBeTruthy();
    await act(async () => {
      fireEvent.click(olderVersionButton!);
    });

    await waitFor(() => {
      expect(apiFetchMock).toHaveBeenCalledWith("/api/admin/modes/1/history/sha-1");
    });
    await waitFor(() => {
      expect(screen.getByText("Системный промпт")).toBeTruthy();
      expect(screen.getByText(/Восстановить эту версию/)).toBeTruthy();
    });
  });

  it("восстановление требует подтверждения и вызывает onRestored при успехе", async () => {
    const onRestored = vi.fn();
    apiFetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url === "/api/admin/modes/1/history") return versionsResponse;
      if (url === "/api/admin/modes/1/history/sha-1") return { version: snapshot1 };
      if (url === "/api/admin/modes/1/restore/sha-1" && init?.method === "POST") return { ok: true };
      throw new Error("unexpected call " + url);
    });
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);

    await act(async () => {
      render(
        <ModeHistoryPanel
          modeId={1}
          current={{ prompt: "Новый промпт", welcomeMessage: "Привет", criteria: "крит" }}
          onRestored={onRestored}
        />,
      );
    });
    await waitFor(() => expect(screen.getAllByText(/Сохранено/).length).toBe(2));

    const buttons = screen.getAllByRole("button");
    const olderVersionButton = buttons.find((b) => b.textContent?.includes("First Admin"));
    await act(async () => {
      fireEvent.click(olderVersionButton!);
    });
    await waitFor(() => screen.getByText(/Восстановить эту версию/));

    await act(async () => {
      fireEvent.click(screen.getByText(/Восстановить эту версию/));
    });

    expect(confirmSpy).toHaveBeenCalled();
    await waitFor(() => {
      expect(apiFetchMock).toHaveBeenCalledWith("/api/admin/modes/1/restore/sha-1", { method: "POST" });
    });
    await waitFor(() => expect(onRestored).toHaveBeenCalled());
  });

  it("отменённое подтверждение не вызывает restore", async () => {
    apiFetchMock.mockImplementation(async (url: string) => {
      if (url === "/api/admin/modes/1/history") return versionsResponse;
      if (url === "/api/admin/modes/1/history/sha-1") return { version: snapshot1 };
      throw new Error("unexpected call " + url);
    });
    vi.spyOn(window, "confirm").mockReturnValue(false);

    await act(async () => {
      render(
        <ModeHistoryPanel modeId={1} current={{ prompt: "Новый промпт" }} onRestored={vi.fn()} />,
      );
    });
    await waitFor(() => expect(screen.getAllByText(/Сохранено/).length).toBe(2));
    const buttons = screen.getAllByRole("button");
    const olderVersionButton = buttons.find((b) => b.textContent?.includes("First Admin"));
    await act(async () => {
      fireEvent.click(olderVersionButton!);
    });
    await waitFor(() => screen.getByText(/Восстановить эту версию/));

    await act(async () => {
      fireEvent.click(screen.getByText(/Восстановить эту версию/));
    });

    expect(apiFetchMock).not.toHaveBeenCalledWith("/api/admin/modes/1/restore/sha-1", expect.anything());
  });
});
