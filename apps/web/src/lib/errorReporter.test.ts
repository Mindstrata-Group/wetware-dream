import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

function readBlob(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error);
    reader.onload = () => resolve(String(reader.result));
    reader.readAsText(blob);
  });
}

describe("errorReporter", () => {
  beforeEach(() => {
    vi.resetModules();
    window.history.replaceState({}, "", "/chat");
  });

  afterEach(async () => {
    const { resetErrorReporterForTests } = await import("./errorReporter");
    resetErrorReporterForTests();
  });

  it("отправляет browser error через sendBeacon с breadcrumbs и dedupe", async () => {
    const sendBeacon = vi.fn(() => true);
    Object.defineProperty(window.navigator, "sendBeacon", {
      configurable: true,
      value: sendBeacon,
    });

    const { installErrorReporter } = await import("./errorReporter");
    installErrorReporter();

    document.body.innerHTML = `<button id="send">Отправить</button>`;
    document.getElementById("send")?.click();

    window.dispatchEvent(new ErrorEvent("error", {
      message: "Cannot read x",
      error: new Error("Cannot read x"),
    }));
    window.dispatchEvent(new ErrorEvent("error", {
      message: "Cannot read x",
      error: new Error("Cannot read x"),
    }));

    expect(sendBeacon).toHaveBeenCalledTimes(1);
    const [url, blob] = sendBeacon.mock.calls[0];
    expect(url).toBe("/api/_error");
    const payload = JSON.parse(await readBlob(blob as Blob));
    expect(payload).toMatchObject({
      type: "error",
      message: "Cannot read x",
      page: "/chat",
    });
    expect(payload.stack).toContain("Cannot read x");
    expect(payload.breadcrumbs.some((crumb: { type: string }) => crumb.type === "click")).toBe(true);
  });

  it("падает на fetch когда sendBeacon возвращает false (убивает sendBeacon && ветку)", async () => {
    Object.defineProperty(window.navigator, "sendBeacon", {
      configurable: true,
      value: vi.fn(() => false), // beacon was not delivered
    });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));

    const { installErrorReporter } = await import("./errorReporter");
    installErrorReporter();

    window.dispatchEvent(new ErrorEvent("error", {
      message: "beacon failed",
      error: new Error("beacon failed"),
    }));

    await new Promise(r => setTimeout(r, 20));
    expect(fetchMock).toHaveBeenCalledWith("/api/_error", expect.objectContaining({ method: "POST" }));
  });

  it("использует fetch когда sendBeacon недоступен (убивает navigator.sendBeacon && мутацию)", async () => {
    Object.defineProperty(window.navigator, "sendBeacon", { configurable: true, value: undefined });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));

    const { installErrorReporter } = await import("./errorReporter");
    installErrorReporter();

    window.dispatchEvent(new ErrorEvent("error", { message: "no beacon", error: new Error("no beacon") }));

    await new Promise(r => setTimeout(r, 20));
    expect(fetchMock).toHaveBeenCalledWith("/api/_error", expect.objectContaining({ method: "POST" }));
  });

  it("не ставит повторные listeners при повторном mount", async () => {
    const sendBeacon = vi.fn(() => true);
    Object.defineProperty(window.navigator, "sendBeacon", {
      configurable: true,
      value: sendBeacon,
    });

    const { installErrorReporter } = await import("./errorReporter");
    installErrorReporter();
    installErrorReporter();

    const event = new Event("unhandledrejection");
    Object.defineProperty(event, "reason", {
      configurable: true,
      value: new Error("network down"),
    });
    window.dispatchEvent(event);

    expect(sendBeacon).toHaveBeenCalledTimes(1);
    const payload = JSON.parse(await readBlob(sendBeacon.mock.calls[0][1] as Blob));
    expect(payload).toMatchObject({
      type: "unhandledrejection",
      message: "network down",
      page: "/chat",
    });
  });

  it("обрезает длинные message и stack до лимитов (убивает slice-мутации)", async () => {
    const sendBeacon = vi.fn(() => true);
    Object.defineProperty(window.navigator, "sendBeacon", { configurable: true, value: sendBeacon });

    const { installErrorReporter } = await import("./errorReporter");
    installErrorReporter();

    const longMsg = "x".repeat(600);
    const longErr = new Error(longMsg);
    // Make the stack longer than 400 characters
    Object.defineProperty(longErr, "stack", { value: "Error: " + "s".repeat(500) });

    window.dispatchEvent(new ErrorEvent("error", { message: longMsg, error: longErr }));

    const payload = JSON.parse(await readBlob(sendBeacon.mock.calls[0][1] as Blob));
    expect(payload.message.length).toBeLessThanOrEqual(500)
    expect(payload.stack.length).toBeLessThanOrEqual(400)
  });
});
