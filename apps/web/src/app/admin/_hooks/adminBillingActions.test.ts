import { describe, expect, it } from "vitest";
import { isPlausibleAIModelID, modePatchPayload, promoEditInitialState, promocodeEditPayload, tariffCreatePayload, tariffPatchPayload } from "./adminBillingActions";

describe("modePatchPayload", () => {
  it("отправляет только редактируемые поля режима", () => {
    const payload = modePatchPayload({
      id: 7,
      name: "Логика",
      hidden: false,
      paid: true,
      paidChains: "Логика -> тариф",
      aiModel: "openai/gpt-test",
      modelTemperature: 0.35,
      prompt: "system prompt",
      welcomeMessage: "welcome",
      demoChat: "[]",
      audioEnabled: true,
      criteria: "criteria",
      orchestratorCheckInterval: 5,
      reminderCount: null,
    });

    expect(payload).toEqual({
      name: "Логика",
      prompt: "system prompt",
      welcomeMessage: "welcome",
      aiModel: "openai/gpt-test",
      aiProvider: "vsegpt",
      thinkingMode: "default",
      modelTemperature: 0.35,
      demoChat: "[]",
      hidden: false,
      audioEnabled: true,
      criteria: "criteria",
      orchestratorCheckInterval: 5,
      reminderCount: null,
      aiMaxTokens: 0,
      leadNotifyEnabled: false,
      leadNotifyChatIds: "",
      leadNotifyTelegramIds: "",
      leadNotifyThreshold: 3,
    });
    expect(payload).not.toHaveProperty("id");
    expect(payload).not.toHaveProperty("paid");
    expect(payload).not.toHaveProperty("paidChains");
  });

  it("прокидывает настройки lead-уведомлений режима", () => {
    const payload = modePatchPayload({
      id: 7,
      name: "Интервьюер",
      hidden: false,
      aiModel: "claude-sonnet-5",
      modelTemperature: 0.7,
      prompt: "p",
      demoChat: "[]",
      leadNotifyEnabled: true,
      leadNotifyChatIds: "111222333, 42",
      leadNotifyTelegramIds: "123456789, @channel",
      leadNotifyThreshold: 4,
    });
    expect(payload).toMatchObject({
      leadNotifyEnabled: true,
      leadNotifyChatIds: "111222333, 42",
      leadNotifyTelegramIds: "123456789, @channel",
      leadNotifyThreshold: 4,
    });
  });
});

describe("isPlausibleAIModelID", () => {
  it("принимает model id и отклоняет текст ответа", () => {
    expect(isPlausibleAIModelID("google/gemini-2.5-flash-lite-pre-0925")).toBe(true);
    expect(isPlausibleAIModelID("openai/gpt-4o-mini")).toBe(true);
    expect(isPlausibleAIModelID("Работу я оцениваю на семь. Это не id модели.")).toBe(false);
    expect(isPlausibleAIModelID("google gemini")).toBe(false);
  });
});

describe("tariff payloads", () => {
  it("создаёт payload тарифа с ведущим режимом по первому выбранному режиму", () => {
    const payload = tariffCreatePayload({
      name: "Презентация Стратум",
      description: "demo",
      tariffType: "promo",
      monthlyPrice: "0",
      dailyMessageLimit: "50",
      limitType: "shared",
      groupId: "12",
      availableForSubscription: true,
      modeIds: [29, 1],
      firstModeId: null,
    });

    expect(payload).toEqual({
      name: "Презентация Стратум",
      description: "demo",
      tariffType: "promo",
      monthlyPrice: 0,
      dailyMessageLimit: 50,
      limitType: "shared",
      groupId: 12,
      availableForSubscription: true,
      modeIds: [29, 1],
      firstModeId: 29,
    });
  });

  it("отправляет в PATCH только поля контракта тарифа", () => {
    const payload = tariffPatchPayload({
      id: 7,
      name: "Презентация Стратум",
      description: "demo",
      tariffType: "promo",
      monthlyPrice: 0,
      dailyMessageLimit: 50,
      limitType: "shared",
      groupId: null,
      groupName: "Демо",
      availableForSubscription: true,
      createdAt: "2026-06-23T10:00:00Z",
      archivedAt: null,
      modeIds: [1, 29],
      firstModeId: 29,
    });

    expect(payload).toEqual({
      name: "Презентация Стратум",
      description: "demo",
      tariffType: "promo",
      monthlyPrice: 0,
      dailyMessageLimit: 50,
      limitType: "shared",
      groupId: null,
      availableForSubscription: true,
      modeIds: [1, 29],
      firstModeId: 29,
    });
    expect(payload).not.toHaveProperty("id");
    expect(payload).not.toHaveProperty("createdAt");
    expect(payload).not.toHaveProperty("archivedAt");
    expect(payload).not.toHaveProperty("groupName");
  });
});

describe("promocode edit payloads", () => {
  it("prefills exhausted promocode with reusable link fields", () => {
    const initial = promoEditInitialState({
      id: 9,
      code: "MS-TEST",
      grantsType: "mode",
      targetId: 1,
      targetIds: [1, 29],
      firstModeId: 29,
      maxUses: 3,
      usedCount: 3,
      applyUrl: "/access?promo=MS-TEST",
    }, "2099-01-01");

    expect(initial).toEqual({
      id: 9,
      grantsType: "mode",
      targetIds: [1, 29],
      firstModeId: 29,
      activeTo: "2099-01-01",
      maxUses: "4",
      updateScope: "new_only",
    });
  });

  it("does not silently extend active or unlimited promocodes", () => {
    const active = promoEditInitialState({
      id: 10,
      code: "MS-ACTIVE",
      grantsType: "tariff",
      targetId: 7,
      maxUses: 100,
      usedCount: 12,
      applyUrl: "/access?promo=MS-ACTIVE",
    }, "2099-01-01");
    const unlimited = promoEditInitialState({
      id: 11,
      code: "MS-UNLIMITED",
      grantsType: "tariff",
      targetId: 7,
      maxUses: 0,
      usedCount: 12,
      applyUrl: "/access?promo=MS-UNLIMITED",
    }, "");

    expect(active.maxUses).toBe("100");
    expect(unlimited.maxUses).toBe("");
  });

  it("sends only the PATCH contract fields", () => {
    const payload = promocodeEditPayload({
      id: 9,
      grantsType: "mode",
      targetIds: [29, 29, 1, 0],
      firstModeId: 29,
      activeTo: "2099-01-01",
      maxUses: "10",
      updateScope: "all_activations",
    });

    expect(payload).toEqual({
      activeTo: "2099-01-01",
      maxUses: 10,
      grantsType: "mode",
      targetIds: [29, 1],
      firstModeId: 29,
      updateScope: "all_activations",
    });
    expect(payload).not.toHaveProperty("id");
  });

  it("omits maxUses when admin leaves the limit unchanged", () => {
    const payload = promocodeEditPayload({
      id: 9,
      grantsType: "tariff",
      targetIds: [7],
      firstModeId: 0,
      activeTo: "",
      maxUses: "",
      updateScope: "new_only",
    });

    expect(payload).toEqual({
      activeTo: "",
      grantsType: "tariff",
      targetIds: [7],
      firstModeId: null,
      updateScope: "new_only",
    });
    expect(payload).not.toHaveProperty("maxUses");
  });
});
