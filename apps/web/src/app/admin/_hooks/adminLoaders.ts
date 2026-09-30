import type { SummaryPrompt } from "../types";
import type { AdminPageContextValue } from "../components/AdminPageContext";
import { apiFetch } from "@/lib/api";

type AdminLoadersContext = Pick<AdminPageContextValue, "userMeta" | "modeMeta" | "userQ" | "modeQ" | "tariffSort" | "promoQ" | "promoDateFrom" | "promoDateTo" | "promoMeta" | "setPromoMeta" | "exportOptionsLoaded" | "setProfile" | "setStatus" | "setAdminStats" | "setUsers" | "setUserMeta" | "setModes" | "setModeMeta" | "setTariffs" | "setGroups" | "setPromocodes" | "setExportUsers" | "setExportModes" | "setExportPromocodes" | "setExportOptionsLoaded" | "setSummaryPrompts" | "setExportFilters" | "setModelStats" | "setOrchestrationPrompt" | "setDialogSummaryPrompt" | "setLeadSummaryPrompt" | "setAISettings" | "setGuardrail" | "handleError">;

export function createAdminLoaders(ctx: AdminLoadersContext) {
  const { userMeta, modeMeta, userQ, modeQ, tariffSort, promoQ, promoDateFrom, promoDateTo, promoMeta, setPromoMeta, exportOptionsLoaded, setProfile, setStatus, setAdminStats, setUsers, setUserMeta, setModes, setModeMeta, setTariffs, setGroups, setPromocodes, setExportUsers, setExportModes, setExportPromocodes, setExportOptionsLoaded, setSummaryPrompts, setExportFilters, setModelStats, setOrchestrationPrompt, setDialogSummaryPrompt, setLeadSummaryPrompt, setAISettings, setGuardrail, handleError } = ctx;
  async function loadProfile() {
    try {
      const loadedProfile = await apiFetch<any>("/api/auth/me");
      setProfile(loadedProfile);
      return loadedProfile;
    } catch (e) {
      handleError(e, "Ошибка профиля");
      return null;
    }
  }
  async function loadSystem() {
    try { setStatus(await apiFetch("/api/admin/status")); } catch (e) { handleError(e, "Ошибка статуса"); }
  }
  async function loadAdminStats() {
    try { setAdminStats(await apiFetch("/api/admin/stats")); } catch (e) { handleError(e, "Ошибка статистики"); }
  }
  async function loadUsers(offset = userMeta.offset, limit = userMeta.limit) {
    try {
      const j = await apiFetch<any>(`/api/admin/users?q=${encodeURIComponent(userQ)}&limit=${limit}&offset=${offset}`);
      setUsers(j.users || []);
      setUserMeta({ total: j.total || 0, limit: j.limit || limit, offset: j.offset || 0 });
    } catch (e) { handleError(e, "Ошибка пользователей"); }
  }
  async function loadModes(offset = modeMeta.offset, limit = modeMeta.limit) {
    try {
      const j = await apiFetch<any>(`/api/admin/modes?q=${encodeURIComponent(modeQ)}&limit=${limit}&offset=${offset}`);
      setModes(j.modes || []);
      setModeMeta({ total: j.total || 0, limit: j.limit || limit, offset: j.offset || 0 });
    } catch (e) { handleError(e, "Ошибка режимов"); }
  }
  async function loadTariffs(sort = tariffSort) {
    try {
      setTariffs((await apiFetch<any>(`/api/admin/tariffs?sort=${sort}&includeArchived=true`)).tariffs || []);
    } catch (e) { handleError(e, "Ошибка тарифов"); }
  }
  async function loadGroups() {
    try { setGroups((await apiFetch<any>("/api/admin/tariff-groups")).groups || []); } catch (e) { handleError(e, "Ошибка групп"); }
  }
  async function loadPromocodes(offset = promoMeta.offset, limit = promoMeta.limit) {
    try {
      const qs = new URLSearchParams({ q: promoQ, offset: String(offset), limit: String(limit) });
      if (promoDateFrom) qs.set("dateFrom", promoDateFrom);
      if (promoDateTo) qs.set("dateTo", promoDateTo);
      const j = await apiFetch<any>(`/api/admin/promocodes?${qs}`);
      setPromocodes(j.promocodes || []);
      setPromoMeta({ total: j.total || 0, limit: j.limit || limit, offset: j.offset || 0 });
    } catch (e) { handleError(e, "Ошибка промокодов"); }
  }
  async function loadExportOptions(force = false) {
    if (exportOptionsLoaded && !force) return;
    try {
      const j = await apiFetch<any>("/api/admin/exports/messages?options=true");
      setExportUsers(j.users || []);
      setExportModes(j.modes || []);
      setExportPromocodes(j.promocodes || []);
      setExportOptionsLoaded(true);
    } catch (e) { handleError(e, "Ошибка списков экспорта"); }
  }
  async function loadSummaryPrompts() {
    try {
      const j = await apiFetch<any>("/api/admin/summary-prompts");
      setSummaryPrompts(j.prompts || []);
      const d = (j.prompts || []).find((p: SummaryPrompt) => p.isDefault);
      if (d) setExportFilters((v) => ({ ...v, promptId: String(d.id), customPrompt: d.prompt }));
    } catch (e) { handleError(e, "Ошибка промптов"); }
  }
  async function loadModelStats() {
    try { setModelStats((await apiFetch<any>("/api/admin/modes/model-stats")).models || []); } catch (e) { handleError(e, "Ошибка статистики моделей"); }
  }
  async function loadGuardrail() {
    try {
      const j = await apiFetch<any>("/api/admin/modes/guardrail");
      if (typeof j.text === "string" && j.text.trim()) {
        setGuardrail((value) => ({ ...value, text: j.text }));
      }
    } catch (e) { handleError(e, "Ошибка защитного блока"); }
  }
  async function loadOrchestrationPrompt() {
    try { const j = await apiFetch<any>("/api/admin/orchestration-prompt"); if (j.prompt) setOrchestrationPrompt(j.prompt); } catch (e) { handleError(e, "Ошибка промпта оркестрации"); }
  }
  async function loadDialogSummaryPrompt() {
    try { const j = await apiFetch<any>("/api/admin/dialog-summary-prompt"); if (j.prompt) setDialogSummaryPrompt(j.prompt); } catch (e) { handleError(e, "Ошибка итогового промпта"); }
  }
  async function loadLeadSummaryPrompt() {
    try { const j = await apiFetch<any>("/api/admin/lead-summary-prompt"); if (j.prompt) setLeadSummaryPrompt(j.prompt); } catch (e) { handleError(e, "Ошибка промпта выжимки лида"); }
  }
  async function loadAISettings() {
    try {
      const j = await apiFetch<any>("/api/admin/ai-settings");
      setAISettings({
        summaryModel: j.summaryModel || "openai/gpt-4o-mini",
        summaryTemperature: String(j.summaryTemperature ?? "0.3"),
        orchestrationModel: j.orchestrationModel || "openai/gpt-4o-mini",
        orchestrationTemperature: String(j.orchestrationTemperature ?? "0.1"),
        orchestrationHistoryLimit: String(j.orchestrationHistoryLimit ?? "12"),
        leadSummaryModel: j.leadSummaryModel || "openai/gpt-4o-mini",
        leadSummaryTemperature: String(j.leadSummaryTemperature ?? "0.3"),
        chatHistoryLimit: String(j.chatHistoryLimit ?? "10"),
        chatMessageMaxChars: String(j.chatMessageMaxChars ?? "30720"),
        retryAttempts: String(j.retryAttempts ?? "3"),
        retryInitialDelayMs: String(j.retryInitialDelayMs ?? "1500"),
        queueEnabled: String(j.queueEnabled ?? "1"),
        queueIntervalMs: String(j.queueIntervalMs ?? "1100"),
        queueTimeoutMs: String(j.queueTimeoutMs ?? "60000"),
        queueConcurrency: String(j.queueConcurrency ?? "1"),
        attachmentDirectMaxBytes: String(j.attachmentDirectMaxBytes ?? "24576"),
        attachmentMaxUploadBytes: String(j.attachmentMaxUploadBytes ?? "5242880"),
        attachmentContextMaxChars: String(j.attachmentContextMaxChars ?? "24000"),
        attachmentAnnotationModel: j.attachmentAnnotationModel || "openai/gpt-4o-mini",
        attachmentAnnotationPrompt: j.attachmentAnnotationPrompt || "Сожми вложенный файл для ответа в чате: сохрани факты, числа, термины, требования и явные вопросы. Не добавляй новых фактов.",
        providerVsegptEnabled: j.providers?.vsegpt?.enabled !== false,
        providerGeminiEnabled: j.providers?.gemini?.enabled !== false,
        providerAnthropicEnabled: j.providers?.anthropic?.enabled !== false,
        providerGeminiConfigured: Boolean(j.providers?.gemini?.configured),
        providerAnthropicConfigured: Boolean(j.providers?.anthropic?.configured),
        providerVsegptConfigured: Boolean(j.providers?.vsegpt?.configured),
        providerGeminiDefaultModel: j.providers?.gemini?.defaultModel || "gemini-3.1-flash-lite",
        providerAnthropicDefaultModel: j.providers?.anthropic?.defaultModel || "claude-haiku-4-5",
        providerVsegptDefaultModel: j.providers?.vsegpt?.defaultModel || "google/gemini-3.1-flash-lite-thinking",
        providerVsegptApiKeyMasked: j.providers?.vsegpt?.apiKeyMasked || "",
        providerGeminiApiKeyMasked: j.providers?.gemini?.apiKeyMasked || "",
        providerAnthropicApiKeyMasked: j.providers?.anthropic?.apiKeyMasked || "",
        providerVsegptApiKeySetFromAdmin: Boolean(j.providers?.vsegpt?.apiKeySetFromAdmin),
        providerGeminiApiKeySetFromAdmin: Boolean(j.providers?.gemini?.apiKeySetFromAdmin),
        providerAnthropicApiKeySetFromAdmin: Boolean(j.providers?.anthropic?.apiKeySetFromAdmin),
        orchestrationProvider: j.orchestrationProvider || "vsegpt",
        leadSummaryProvider: j.leadSummaryProvider || "vsegpt",
        summaryProvider: j.summaryProvider || "vsegpt",
        attachmentAnnotationProvider: j.attachmentAnnotationProvider || "vsegpt",
      });
    } catch (e) { handleError(e, "Ошибка настроек AI"); }
  }


  return { loadProfile, loadSystem, loadAdminStats, loadUsers, loadModes, loadTariffs, loadGroups, loadPromocodes, loadExportOptions, loadSummaryPrompts, loadModelStats, loadGuardrail, loadOrchestrationPrompt, loadDialogSummaryPrompt, loadLeadSummaryPrompt, loadAISettings };
}
