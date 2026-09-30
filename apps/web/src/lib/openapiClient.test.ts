import { afterEach, describe, expect, it, vi } from "vitest";
import { createYooKassaPayment, getAccessStatus, getAdminPayments, getAuthMe, getProfile, sendChatMessage, startChat } from "./openapiClient";

describe("openapiClient P0 wrappers", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sendChatMessage sends the documented text payload via POST to /api/chat/send", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({
        ok: true,
        user: { id: 1, role: "user", content: "hi", createdAt: "2026-06-05T00:00:00Z" },
        assistant: { id: 2, role: "assistant", content: "ok", createdAt: "2026-06-05T00:00:01Z" },
      }), { status: 200, headers: { "content-type": "application/json" } }),
    );

    await sendChatMessage({ dialogId: 10, text: "hi", responseMode: "short", knowledgeModeIds: [1], attachmentIds: [7] });

    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toContain("/api/chat/send");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      dialogId: 10,
      text: "hi",
      responseMode: "short",
      knowledgeModeIds: [1],
      attachmentIds: [7],
    });
  });

  it("createYooKassaPayment sends the public payment contract via POST", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({
        ok: true,
        paymentId: "yk-test",
        confirmationUrl: "https://yookassa.ru/checkout/payments/yk-test",
      }), { status: 200, headers: { "content-type": "application/json" } }),
    );

    await createYooKassaPayment({ tariffId: 5, subscriptionMonths: 3, enableAutoRenew: true });

    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toContain("/api/payments/yookassa/create");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      tariffId: 5,
      subscriptionMonths: 3,
      enableAutoRenew: true,
    });
  });

  it.each([
    ["getAuthMe", getAuthMe, "/api/auth/me"],
    ["getProfile", getProfile, "/api/profile"],
    ["getAccessStatus", getAccessStatus, "/api/access/status"],
    ["getAdminPayments", getAdminPayments, "/api/admin/payments"],
  ] as const)("%s hits the correct path with GET", async (_name, fn, path) => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "content-type": "application/json" } }),
    );
    await (fn as () => Promise<unknown>)();
    expect(fetchMock.mock.calls[0][0]).toContain(path);
    const init = fetchMock.mock.calls[0][1] as RequestInit | undefined;
    expect(init?.method).toBeUndefined(); // GET: the method is not passed explicitly
  });

  it("startChat sends POST with no body", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ ok: true, dialogId: 5 }), { status: 200, headers: { "content-type": "application/json" } }),
    );
    await startChat();
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toContain("/api/chat/start");
    expect((init as RequestInit).method).toBe("POST");
  });
});
