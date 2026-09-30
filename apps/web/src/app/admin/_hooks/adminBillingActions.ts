import type { CreatedPromo, ModeRow, Promo, Tariff } from "../types";
import type { Dispatch, SetStateAction } from "react";
import type { AdminPageContextValue, PromoEditForm } from "../components/AdminPageContext";
import { apiFetch } from "@/lib/api";
import { monthAheadLocalDate, yesterdayLocalDate } from "../utils";

type AdminBillingActionsContext = Pick<AdminPageContextValue, "modeDetail" | "setModeDetail" | "newMode" | "setNewMode" | "deleteModeHoldTimer" | "deleteModeHoldTriggered" | "guardrail" | "modes" | "tariffForm" | "selectedTariff" | "groupForm" | "selectedGroup" | "promoCreating" | "promoForm" | "setError" | "setLastCreatedPromos" | "setPromoForm" | "setNotice" | "tariffs" | "setSelectedTariff" | "setSelectedTariffIds" | "selectedTariffIds" | "setSelectedGroup" | "promoActivation" | "setPromoActivation" | "promoEdit" | "setPromoEdit" | "bulkPromoDeactivate" | "orchestrationPrompt" | "dialogSummaryPrompt" | "leadSummaryPrompt" | "aiSettings" | "showNotice" | "handleError" | "loadModes" | "loadExportOptions" | "loadModelStats" | "loadTariffs" | "loadGroups" | "loadPromocodes"> & { setPromoCreating: Dispatch<SetStateAction<boolean>> };

export function modePatchPayload(modeDetail: AdminPageContextValue["modeDetail"]) {
  if (!modeDetail) return null;
  return {
    name: modeDetail.name,
    prompt: modeDetail.prompt,
    welcomeMessage: modeDetail.welcomeMessage ?? "",
    aiModel: modeDetail.aiModel,
    aiProvider: modeDetail.aiProvider ?? "vsegpt",
    thinkingMode: modeDetail.thinkingMode ?? "default",
    modelTemperature: modeDetail.modelTemperature,
    demoChat: modeDetail.demoChat,
    hidden: modeDetail.hidden,
    audioEnabled: Boolean(modeDetail.audioEnabled),
    criteria: modeDetail.criteria ?? "",
    orchestratorCheckInterval: modeDetail.orchestratorCheckInterval,
    reminderCount: modeDetail.reminderCount ?? null,
    aiMaxTokens: modeDetail.aiMaxTokens ?? 0,
    leadNotifyEnabled: Boolean(modeDetail.leadNotifyEnabled),
    leadNotifyChatIds: modeDetail.leadNotifyChatIds ?? "",
    leadNotifyTelegramIds: modeDetail.leadNotifyTelegramIds ?? "",
    leadNotifyThreshold: modeDetail.leadNotifyThreshold ?? 3,
  };
}

function uniquePositiveNumbers(ids: number[] | undefined) {
  return Array.from(new Set((ids || []).filter((id) => Number.isFinite(id) && id > 0)));
}

function resolvePayloadFirstModeID(firstModeId: number | null | undefined, modeIds: number[]) {
  if (!modeIds.length) return null;
  return firstModeId && modeIds.includes(firstModeId) ? firstModeId : modeIds[0];
}

export function tariffCreatePayload(tariffForm: AdminPageContextValue["tariffForm"]) {
  const modeIds = uniquePositiveNumbers(tariffForm.modeIds);
  return {
    name: tariffForm.name,
    description: tariffForm.description,
    tariffType: tariffForm.tariffType,
    monthlyPrice: Number(tariffForm.monthlyPrice),
    dailyMessageLimit: Number(tariffForm.dailyMessageLimit),
    limitType: tariffForm.limitType,
    groupId: tariffForm.groupId ? Number(tariffForm.groupId) : null,
    availableForSubscription: tariffForm.availableForSubscription,
    modeIds,
    firstModeId: resolvePayloadFirstModeID(tariffForm.firstModeId, modeIds),
  };
}

export function tariffPatchPayload(tariff: Tariff) {
  const modeIds = uniquePositiveNumbers(tariff.modeIds);
  return {
    name: tariff.name,
    description: tariff.description ?? "",
    tariffType: tariff.tariffType || "regular",
    monthlyPrice: Number(tariff.monthlyPrice) || 0,
    dailyMessageLimit: tariff.dailyMessageLimit ?? null,
    limitType: tariff.limitType || "shared",
    groupId: tariff.groupId ?? null,
    availableForSubscription: tariff.availableForSubscription,
    modeIds,
    firstModeId: resolvePayloadFirstModeID(tariff.firstModeId, modeIds),
  };
}

export function promoEditInitialState(promo: Promo, activeTo: string): PromoEditForm {
  const targetIds = uniquePositiveNumbers(promo.targetIds?.length ? promo.targetIds : [promo.targetId]);
  const maxUses = promo.maxUses || 0;
  const usedCount = promo.usedCount || 0;
  const nextMaxUses = maxUses > 0
    ? String(usedCount >= maxUses ? Math.max(maxUses + 1, usedCount + 1) : maxUses)
    : "";
  return {
    id: promo.id,
    grantsType: promo.grantsType || "tariff",
    targetIds,
    firstModeId: promo.firstModeId || 0,
    activeTo,
    maxUses: nextMaxUses,
    updateScope: "new_only",
  };
}

export function promocodeEditPayload(edit: PromoEditForm) {
  const payload: Record<string, unknown> = {
    activeTo: edit.activeTo,
    grantsType: edit.grantsType,
    targetIds: uniquePositiveNumbers(edit.targetIds),
    firstModeId: edit.firstModeId > 0 ? edit.firstModeId : null,
    updateScope: edit.updateScope,
  };
  if (edit.maxUses.trim() !== "") {
    payload.maxUses = Number(edit.maxUses) || 0;
  }
  return payload;
}

export function isPlausibleAIModelID(model: string) {
  return /^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,159}$/.test(model.trim());
}

export function createAdminBillingActions(ctx: AdminBillingActionsContext) {
  const { modeDetail, setModeDetail, newMode, setNewMode, deleteModeHoldTimer, deleteModeHoldTriggered, guardrail, modes, tariffForm, selectedTariff, groupForm, selectedGroup, promoCreating, promoForm, setPromoCreating, setError, setLastCreatedPromos, setPromoForm, setNotice, tariffs, setSelectedTariff, setSelectedTariffIds, selectedTariffIds, setSelectedGroup, promoActivation, setPromoActivation, promoEdit, setPromoEdit, bulkPromoDeactivate, orchestrationPrompt, dialogSummaryPrompt, leadSummaryPrompt, aiSettings, showNotice, handleError, loadModes, loadExportOptions, loadModelStats, loadTariffs, loadGroups, loadPromocodes } = ctx;
  async function openMode(mode: ModeRow) {
    try { setModeDetail((await apiFetch<any>(`/api/admin/modes/${mode.id}`)).mode); } catch (e) { handleError(e, "Ошибка режима"); }
  }
  async function saveMode(e: React.FormEvent) {
    e.preventDefault(); if (!modeDetail) return;
    if (!isPlausibleAIModelID(modeDetail.aiModel)) { showNotice("Модель должна быть коротким ID без пробелов: например google/gemini-2.5-flash-lite-pre-0925."); return; }
    try { await apiFetch(`/api/admin/modes/${modeDetail.id}`, { method: "PATCH", body: JSON.stringify(modePatchPayload(modeDetail)) }); showNotice("Режим сохранён."); await loadModes(); } catch (err) { handleError(err, "Ошибка режима"); }
  }
  async function copyMode() {
    if (!modeDetail) return;
    try {
      const json = await apiFetch<any>(`/api/admin/modes/${modeDetail.id}/copy`, { method: "POST" });
      showNotice("Режим скопирован (скрыт).");
      await loadModes(0);
      await loadExportOptions(true);
      if (json.modeId) await openMode({ id: json.modeId, name: "Копия " + modeDetail.name, hidden: true, aiModel: modeDetail.aiModel });
    } catch (err) { handleError(err, "Ошибка копирования режима"); }
  }
  async function createMode(e: React.FormEvent) {
    e.preventDefault();
    if (!isPlausibleAIModelID(newMode.aiModel)) { showNotice("Модель должна быть коротким ID без пробелов: например google/gemini-2.5-flash-lite-pre-0925."); return; }
    try {
      const json = await apiFetch<any>("/api/admin/modes", { method: "POST", body: JSON.stringify({ ...newMode, modelTemperature: Number(newMode.modelTemperature), orchestratorCheckInterval: Number(newMode.orchestratorCheckInterval), reminderCount: Number(newMode.reminderCount), demoChat: "[]" }) });
      showNotice("Режим создан.");
      setNewMode({ name: "", prompt: "", welcomeMessage: "", aiModel: "openai/gpt-4o-mini", modelTemperature: "0.7", criteria: "", orchestratorCheckInterval: "5", reminderCount: "0", hidden: false, audioEnabled: false });
      await loadModes(0);
      await loadExportOptions(true);
      if (json.modeId) await openMode({ id: json.modeId, name: newMode.name, hidden: false, aiModel: newMode.aiModel });
    } catch (err) { handleError(err, "Ошибка создания режима"); }
  }
  async function testModeModel() {
    if (!modeDetail) return;
    if (!isPlausibleAIModelID(modeDetail.aiModel)) { showNotice("Модель должна быть коротким ID без пробелов: например google/gemini-2.5-flash-lite-pre-0925."); return; }
    try {
      const j = await apiFetch<any>(`/api/admin/modes/${modeDetail.id}/test-model`, { method: "POST", body: JSON.stringify({ model: modeDetail.aiModel, temperature: modeDetail.modelTemperature, text: "Привет" }) });
      showNotice(`Тест модели OK: ${j.model}. Ответ: ${j.response || j.message || "ok"}`);
    } catch (err) { handleError(err, "Ошибка теста модели"); }
  }
  async function detachModeFromPaidTariffs() {
    if (deleteModeHoldTriggered.current) { deleteModeHoldTriggered.current = false; return; }
    if (!modeDetail) return;
    try {
      const json = await apiFetch<any>(`/api/admin/modes/${modeDetail.id}/detach-paid-tariffs`, { method: "POST" });
      showNotice(`Режим убран из доступных для подписки платных тарифов: ${json.removed || 0}.`);
      await loadModes();
    } catch (err) { handleError(err, "Ошибка платных тарифов"); }
  }
  function cancelDeleteModeHold() {
    if (!deleteModeHoldTimer.current) return;
    clearTimeout(deleteModeHoldTimer.current); deleteModeHoldTimer.current = null;
  }
  function startDeleteModeHold() {
    cancelDeleteModeHold(); if (!modeDetail) return;
    deleteModeHoldTimer.current = setTimeout(() => {
      deleteModeHoldTimer.current = null; deleteModeHoldTriggered.current = true;
      void deleteModeFromDatabase();
    }, 1600);
  }
  async function deleteModeFromDatabase() {
    if (!modeDetail) return;
    const modeName = modeDetail.name;
    if (!window.confirm(`Секретное удаление: навсегда удалить режим «${modeName}» из базы? Если режим уже связан с историями или доступами, сервер не даст удалить.`)) return;
    try {
      await apiFetch(`/api/admin/modes/${modeDetail.id}`, { method: "DELETE" });
      showNotice(`Режим «${modeName}» удалён из базы.`); setModeDetail(null); await loadModes();
    } catch (err) { handleError(err, "Ошибка удаления режима из базы"); }
  }
  async function applyGuardrail() {
    if (guardrail.replaceAll && !isPlausibleAIModelID(guardrail.replacement)) { showNotice("Для массовой замены укажите ID модели без пробелов, не текст промпта или ответа."); return; }
    try {
      const j = await apiFetch<any>("/api/admin/modes/guardrail", { method: "POST", body: JSON.stringify({ text: guardrail.text, replaceAll: guardrail.replaceAll, find: guardrail.find, replacement: guardrail.replacement }) });
      showNotice(j.runtimeAppend ? `Защитный блок сохранён. Замены в режимах: ${j.updated || 0}.` : `Защита обновлена: ${j.updated}`);
      await loadModelStats();
    } catch (e) { handleError(e, "Ошибка guardrail"); }
  }
  async function testGuardrailModel() {
    try {
      const mode = modeDetail || modes[0];
      if (!mode) { showNotice("Сначала создайте или загрузите хотя бы один режим."); return; }
      const testModel = guardrail.replaceAll ? guardrail.replacement.trim() : guardrail.testModel.trim();
      if (!testModel && guardrail.replaceAll) { showNotice("Укажите модель в поле «На что заменить» перед тестом."); return; }
      if (testModel && !isPlausibleAIModelID(testModel)) { showNotice("Модель должна быть коротким ID без пробелов: например google/gemini-2.5-flash-lite-pre-0925."); return; }
      const j = await apiFetch<any>(`/api/admin/modes/${mode.id}/test-model`, { method: "POST", body: JSON.stringify({ model: testModel, text: "Привет", temperature: 0.2 }) });
      showNotice(`Тест защитного блока OK: ${j.model}. Ответ: ${j.response || j.message || "ok"}`);
    } catch (e) { handleError(e, "Ошибка теста защитного блока"); }
  }
  async function createTariff(e: React.FormEvent) {
    e.preventDefault();
    try {
      await apiFetch("/api/admin/tariffs", { method: "POST", body: JSON.stringify(tariffCreatePayload(tariffForm)) });
      showNotice("Тариф создан."); await loadTariffs();
    } catch (err) { handleError(err, "Ошибка тарифа"); }
  }
  async function saveTariff(e: React.FormEvent) {
    e.preventDefault(); if (!selectedTariff) return;
    try { await apiFetch(`/api/admin/tariffs/${selectedTariff.id}`, { method: "PATCH", body: JSON.stringify(tariffPatchPayload(selectedTariff)) }); showNotice("Тариф сохранён."); await loadTariffs(); } catch (err) { handleError(err, "Ошибка тарифа"); }
  }
  // Bulk change of provider and model for ALL modes of a tariff:
  // one request instead of editing each mode by hand.
  async function applyTariffModesAI(provider: string, model: string) {
    if (!selectedTariff) return;
    if (!isPlausibleAIModelID(model)) { showNotice("Модель должна быть коротким ID без пробелов: например claude-haiku-4-5."); return; }
    const modeCount = (selectedTariff.modeIds || []).length;
    if (!modeCount) { showNotice("У тарифа нет режимов — нечего переключать."); return; }
    if (!window.confirm(`Переключить ВСЕ режимы тарифа «${selectedTariff.name}» (${modeCount}) на ${provider} / ${model}? Затронет и другие тарифы, где используются эти же режимы.`)) return;
    try {
      const j = await apiFetch<any>(`/api/admin/tariffs/${selectedTariff.id}/set-modes-ai`, { method: "POST", body: JSON.stringify({ provider, model }) });
      showNotice(`Провайдер и модель применены. Режимов обновлено: ${j.updated ?? 0}.`);
      await loadModes();
    } catch (err) { handleError(err, "Ошибка массовой смены модели режимов"); }
  }
  async function createGroup(e: React.FormEvent) {
    e.preventDefault();
    try { await apiFetch("/api/admin/tariff-groups", { method: "POST", body: JSON.stringify(groupForm) }); showNotice("Группа создана."); await loadGroups(); } catch (err) { handleError(err, "Ошибка группы"); }
  }
  async function saveGroup(e: React.FormEvent) {
    e.preventDefault(); if (!selectedGroup) return;
    try { await apiFetch(`/api/admin/tariff-groups/${selectedGroup.id}`, { method: "PATCH", body: JSON.stringify(selectedGroup) }); showNotice("Группа сохранена."); await loadGroups(); } catch (err) { handleError(err, "Ошибка группы"); }
  }
  async function createPromocodes(e: React.FormEvent) {
    e.preventDefault(); if (promoCreating) return;
    setPromoCreating(true); setError("");
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 20000);
    try {
      const summary = promoForm.summaryLimit === "∞" ? null : Number(promoForm.summaryLimit);
      const j = await apiFetch<{ promocodes?: CreatedPromo[] }>("/api/admin/promocodes", { method: "POST", signal: controller.signal, body: JSON.stringify({ ...promoForm, autoGenerate: true, bulkCount: Number(promoForm.bulkCount), durationDays: Number(promoForm.durationDays), maxUses: Number(promoForm.maxUses), dailyMessageLimit: Number(promoForm.dailyMessageLimit), summaryLimit: summary }) });
      const createdPromos = j.promocodes || [];
      setLastCreatedPromos(createdPromos);
      setPromoForm((v) => ({ ...v, activeFrom: yesterdayLocalDate(), activeTo: monthAheadLocalDate() }));
      showNotice(createdPromos.length > 1 ? `Создан пакет промокодов: ${createdPromos.length}.` : "Промокод создан.");
      void loadPromocodes();
    } catch (err) {
      setNotice(""); setError(`Проверьте поля дат, длительности, лимитов и выбранный тариф/режим. ${err instanceof Error ? err.message : ""}`);
    } finally { window.clearTimeout(timeout); setPromoCreating(false); }
  }
  async function restoreTariff(tariff: Tariff) {
    try { await apiFetch(`/api/admin/tariffs/${tariff.id}`, { method: "PATCH", body: JSON.stringify(tariffPatchPayload({ ...tariff, availableForSubscription: true })) }); setSelectedTariff(null); showNotice(`Тариф «${tariff.name}» восстановлен из архива.`); await loadTariffs(); } catch (e) { handleError(e, "Ошибка восстановления тарифа"); }
  }
  async function deleteTariff(id: number, permanent = false) {
    const tariff = tariffs.find((item) => item.id === id) || selectedTariff;
    const isArchived = Boolean(tariff?.archivedAt || tariff?.availableForSubscription === false);
    const shouldDeletePermanently = permanent || isArchived;
    if (!window.confirm(shouldDeletePermanently ? "Удалить тариф из архива окончательно?" : "Перенести тариф в архив?")) return;
    try {
      await apiFetch(`/api/admin/tariffs/${id}`, { method: "DELETE" });
      setSelectedTariff(null); setSelectedTariffIds((ids) => ids.filter((item) => item !== id));
      showNotice(shouldDeletePermanently ? "Тариф окончательно удалён из архива." : "Тариф перенесён в архив.");
      await loadTariffs();
    } catch (e) { handleError(e, shouldDeletePermanently ? "Ошибка окончательного удаления тарифа" : "Ошибка архивации тарифа"); }
  }
  async function deleteSelectedTariffs() {
    if (!selectedTariffIds.length) return;
    if (!window.confirm(`Перенести выбранные тарифы (${selectedTariffIds.length}) в архив?`)) return;
    try {
      await Promise.all(selectedTariffIds.map((id) => apiFetch(`/api/admin/tariffs/${id}`, { method: "DELETE" })));
      setSelectedTariff(null); setSelectedTariffIds([]); showNotice("Выбранные тарифы перенесены в архив."); await loadTariffs();
    } catch (e) { handleError(e, "Ошибка массового удаления тарифов"); }
  }
  async function deleteGroup(id: number, name: string) {
    if (!window.confirm(`Удалить группу «${name}»? Базовую группу участников удалять нельзя.`)) return;
    try {
      await apiFetch(`/api/admin/tariff-groups/${id}`, { method: "DELETE" });
      setSelectedGroup(null); showNotice("Группа удалена."); await Promise.all([loadGroups(), loadTariffs()]);
    } catch (e) { handleError(e, "Ошибка удаления группы"); }
  }
  async function deactivatePromocode(id: number) {
    try { await apiFetch(`/api/admin/promocodes/${id}`, { method: "DELETE" }); await loadPromocodes(); } catch (e) { handleError(e, "Ошибка деактивации"); }
  }
  async function activatePromocode() {
    if (!promoActivation) return;
    try { await apiFetch(`/api/admin/promocodes/${promoActivation.id}`, { method: "PATCH", body: JSON.stringify({ activeTo: promoActivation.activeTo, maxUses: Number(promoActivation.maxUses) || 0 }) }); setPromoActivation(null); showNotice("Промокод снова активен."); await loadPromocodes(); } catch (e) { handleError(e, "Ошибка активации промокода"); }
  }
  async function savePromocodeEdit() {
    if (!promoEdit) return;
    try {
      const j = await apiFetch<any>(`/api/admin/promocodes/${promoEdit.id}`, { method: "PATCH", body: JSON.stringify(promocodeEditPayload(promoEdit)) });
      setPromoEdit(null);
      const synced = Number(j.updatedActivations || 0) + Number(j.deactivatedAccesses || 0) + Number(j.roleUpgrades || 0);
      showNotice(promoEdit.updateScope === "all_activations" ? `Промокод обновлён. Изменений в доступах: ${synced}.` : "Промокод обновлён для новых активаций.");
      await loadPromocodes();
    } catch (e) { handleError(e, "Ошибка обновления промокода"); }
  }
  async function bulkDeactivatePromocodes() {
    const scoped = bulkPromoDeactivate.dateFrom || bulkPromoDeactivate.dateTo;
    if (!window.confirm(scoped ? "Деактивировать активные промокоды в выбранном диапазоне дат?" : "Даты не выбраны: будут деактивированы ВСЕ активные промокоды. Продолжить?")) return;
    try {
      const j = await apiFetch<any>("/api/admin/promocodes/bulk-deactivate", { method: "POST", body: JSON.stringify(bulkPromoDeactivate) });
      showNotice(`Деактивировано промокодов: ${j.updated || 0}.`); await loadPromocodes();
    } catch (e) { handleError(e, "Ошибка массовой деактивации промокодов"); }
  }
  async function saveOrchestrationPrompt() {
    try { await apiFetch("/api/admin/orchestration-prompt", { method: "POST", body: JSON.stringify({ prompt: orchestrationPrompt }) }); showNotice("Единый промпт оркестрации сохранён."); } catch (e) { handleError(e, "Ошибка оркестрации"); }
  }
  async function saveDialogSummaryPrompt() {
    try { await apiFetch("/api/admin/dialog-summary-prompt", { method: "POST", body: JSON.stringify({ prompt: dialogSummaryPrompt }) }); showNotice("Итоговый промпт завершения диалога сохранён."); } catch (e) { handleError(e, "Ошибка итогового промпта"); }
  }
  async function saveLeadSummaryPrompt() {
    try { await apiFetch("/api/admin/lead-summary-prompt", { method: "POST", body: JSON.stringify({ prompt: leadSummaryPrompt }) }); showNotice("Промпт выжимки лида сохранён."); } catch (e) { handleError(e, "Ошибка промпта выжимки лида"); }
  }
  // newApiKeys: new key values from the form (not stored in aiSettings:
  // the server never returns the real key, only a mask). An empty
  // string/undefined = do not change; the server ignores an empty value.
  async function saveAISettings(newApiKeys?: { vsegpt?: string; gemini?: string; anthropic?: string }) {
    try {
      const {
        providerVsegptEnabled, providerGeminiEnabled, providerAnthropicEnabled,
        providerVsegptConfigured, providerGeminiConfigured, providerAnthropicConfigured,
        providerVsegptApiKeyMasked, providerGeminiApiKeyMasked, providerAnthropicApiKeyMasked,
        providerVsegptApiKeySetFromAdmin, providerGeminiApiKeySetFromAdmin, providerAnthropicApiKeySetFromAdmin,
        ...rest
      } = aiSettings;
      await apiFetch("/api/admin/ai-settings", {
        method: "POST",
        body: JSON.stringify({
          ...rest,
          providerVsegptEnabled: providerVsegptEnabled ? "1" : "0",
          providerGeminiEnabled: providerGeminiEnabled ? "1" : "0",
          providerAnthropicEnabled: providerAnthropicEnabled ? "1" : "0",
          providerVsegptApiKey: newApiKeys?.vsegpt || undefined,
          providerGeminiApiKey: newApiKeys?.gemini || undefined,
          providerAnthropicApiKey: newApiKeys?.anthropic || undefined,
        }),
      });
      showNotice("Настройки AI сохранены.");
    } catch (e) { handleError(e, "Ошибка настроек AI"); }
  }

  return { openMode, saveMode, copyMode, createMode, testModeModel, detachModeFromPaidTariffs, cancelDeleteModeHold, startDeleteModeHold, deleteModeFromDatabase, applyGuardrail, testGuardrailModel, createTariff, saveTariff, applyTariffModesAI, createGroup, saveGroup, createPromocodes, savePromocodeEdit, restoreTariff, deleteTariff, deleteSelectedTariffs, deleteGroup, deactivatePromocode, activatePromocode, bulkDeactivatePromocodes, saveOrchestrationPrompt, saveDialogSummaryPrompt, saveLeadSummaryPrompt, saveAISettings };
}
