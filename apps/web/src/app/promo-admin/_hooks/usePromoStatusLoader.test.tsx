import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { usePromoStatusLoader } from "./usePromoStatusLoader";

// apiFetch uses res.text() rather than res.json(), so a
// real Response with the correct content-type is needed.
const jsonResponse = (payload: unknown) =>
  new Response(JSON.stringify(payload), {
    status: 200,
    headers: { "content-type": "application/json" },
  });

const okStatus = (promo: Record<string, unknown>) => ({
  ok: true,
  promo,
  prompts: [{ id: 1, name: "default", prompt: "p", isDefault: true }],
  modes: [{ id: 7, name: "Test mode" }],
  history: [],
});

describe("usePromoStatusLoader", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;
  const setError = vi.fn();

  beforeEach(() => {
    fetchSpy = vi.fn(async () =>
      jsonResponse(okStatus({ messageCount: 5, summaryUsed: 0 })),
    );
    vi.stubGlobal("fetch", fetchSpy);
    setError.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("initial load: fetch один раз и пишет status (без fake timers)", async () => {
    const { result } = renderHook(() =>
      usePromoStatusLoader("PROMO1", "key123", setError),
    );

    await waitFor(() => expect(result.current.status?.messageCount).toBe(5));
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    expect(result.current.promptId).toBe(1);
    expect(result.current.loadingStatus).toBe(false);
  });

  it("auto-refresh: после 60 секунд делает silent fetch", async () => {
    // waitFor v10 uses setInterval for polling; fake timers break it.
    // Intercept specifically the hook's 60-second interval via spyOn;
    // all other setInterval calls (waitFor, React) go through the real one.
    let autoRefreshCb: (() => void) | null = null;
    const realSetInterval = window.setInterval.bind(window);
    vi.spyOn(window, "setInterval").mockImplementation(
      (fn: TimerHandler, delay?: number, ...args: unknown[]) => {
        if (delay === 60_000 && !autoRefreshCb) {
          autoRefreshCb = fn as () => void;
          return 999 as unknown as ReturnType<typeof setInterval>;
        }
        return realSetInterval(fn, delay, ...(args as []));
      },
    );

    const { result } = renderHook(() =>
      usePromoStatusLoader("PROMO1", "key123", setError),
    );
    await waitFor(() => expect(result.current.status).not.toBeNull());
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    expect(autoRefreshCb).not.toBeNull();

    fetchSpy.mockResolvedValueOnce(
      jsonResponse(okStatus({ messageCount: 10, summaryUsed: 2 })),
    );
    await act(async () => {
      autoRefreshCb!();
    });

    await waitFor(() => expect(result.current.status?.messageCount).toBe(10));
    expect(fetchSpy).toHaveBeenCalledTimes(2);
    expect(result.current.loadingStatus).toBe(false);
  });

  it("при document.hidden пропускает тик авто-обновления", async () => {
    let autoRefreshCb: (() => void) | null = null;
    const realSetInterval = window.setInterval.bind(window);
    vi.spyOn(window, "setInterval").mockImplementation(
      (fn: TimerHandler, delay?: number, ...args: unknown[]) => {
        if (delay === 60_000 && !autoRefreshCb) {
          autoRefreshCb = fn as () => void;
          return 999 as unknown as ReturnType<typeof setInterval>;
        }
        return realSetInterval(fn, delay, ...(args as []));
      },
    );

    const { result } = renderHook(() =>
      usePromoStatusLoader("PROMO1", "key123", setError),
    );
    await waitFor(() => expect(result.current.status).not.toBeNull());
    expect(autoRefreshCb).not.toBeNull();

    Object.defineProperty(document, "hidden", { configurable: true, get: () => true });
    await act(async () => { autoRefreshCb!(); });
    expect(fetchSpy).toHaveBeenCalledTimes(1);

    Object.defineProperty(document, "hidden", { configurable: true, get: () => false });
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(okStatus({ messageCount: 7, summaryUsed: 0 })),
    );
    await act(async () => { autoRefreshCb!(); });
    await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(2));
  });

  it("без promo → setError, без fetch", async () => {
    renderHook(() => usePromoStatusLoader("", "key", setError));
    await act(async () => {
      await Promise.resolve();
    });
    expect(setError).toHaveBeenCalledWith(expect.stringContaining("promo"));
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("без key → setError, без fetch", async () => {
    renderHook(() => usePromoStatusLoader("PROMO1", "", setError));
    await act(async () => {
      await Promise.resolve();
    });
    expect(setError).toHaveBeenCalledWith(expect.stringContaining("key"));
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
