import type { CreatedPromo, ModeDetail, ModeRow, ModelStat, PageMeta, Promo, SortState, SummaryPrompt, Tab, Tariff, UserDetail, UserRow } from "../types";
import type { PromoEditForm, PromoSection } from "../components/AdminPageContext";
import { useRef, useState } from "react";
import { defaultGuardrail, defaultSummaryPrompt } from "../constants";
import { monthAheadLocalDate, monthBeforeLocalDate, todayLocalDate, weekAgoLocalDate, yesterdayLocalDate } from "../utils";

export function useAdminControllerState() {
  const deleteModeHoldTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const deleteModeHoldTriggered = useRef(false);
  const [tab, setTab] = useState<Tab>("profile");
  const [profile, setProfile] = useState<any>(null);
  const [status, setStatus] = useState<any>(null);
  const [adminStats, setAdminStats] = useState<any>(null);
  const [statsSort, setStatsSort] = useState<Record<string, SortState>>({});
  const [exportSort, setExportSort] = useState<Record<string, SortState>>({});
  const [users, setUsers] = useState<UserRow[]>([]);
  const [userDetail, setUserDetail] = useState<UserDetail | null>(null);
  const [grantUserDetail, setGrantUserDetail] = useState<UserDetail | null>(null);
  const [userMeta, setUserMeta] = useState<PageMeta>({ total: 0, limit: 50, offset: 0 });
  const [modes, setModes] = useState<ModeRow[]>([]);
  const [modeMeta, setModeMeta] = useState<PageMeta>({ total: 0, limit: 50, offset: 0 });
  const [modeDetail, setModeDetail] = useState<ModeDetail | null>(null);
  const [tariffs, setTariffs] = useState<Tariff[]>([]);
  const [groups, setGroups] = useState<any[]>([]);
  const [selectedTariff, setSelectedTariff] = useState<Tariff | null>(null);
  const [selectedTariffIds, setSelectedTariffIds] = useState<number[]>([]);
  const [tariffArchiveView, setTariffArchiveView] = useState<"active" | "archive">("active");
  const [modePanel, setModePanel] = useState<"edit" | "create" | "guardrail">("edit");
  const [promocodes, setPromocodes] = useState<Promo[]>([]);
  const [lastCreatedPromos, setLastCreatedPromos] = useState<CreatedPromo[]>([]);
  const [summaryPrompts, setSummaryPrompts] = useState<SummaryPrompt[]>([]);
  const [exportResult, setExportResult] = useState<any[]>([]);
  const [exportSummaryPayload, setExportSummaryPayload] = useState("");
  const [exportSummaryResult, setExportSummaryResult] = useState("");
  const [exportSummaryHistory, setExportSummaryHistory] = useState<any[]>([]);
  const [exportMeta, setExportMeta] = useState({ messageCount: 0, sourceBytes: 0, approxTokens: 0 });
  const [userQ, setUserQ] = useState("");
  const [modeQ, setModeQ] = useState("");
  const [promoQ, setPromoQ] = useState("");
  const [promoDateFrom, setPromoDateFrom] = useState("");
  const [promoDateTo, setPromoDateTo] = useState("");
  const [promoMeta, setPromoMeta] = useState<PageMeta>({ total: 0, limit: 100, offset: 0 });
  const [promoSection, setPromoSection] = useState<PromoSection>("active");
  const [promoActivation, setPromoActivation] = useState<{ id: number; activeTo: string; maxUses: string } | null>(null);
  const [promoEdit, setPromoEdit] = useState<PromoEditForm | null>(null);
  const [bulkPromoDeactivate, setBulkPromoDeactivate] = useState({ dateFrom: weekAgoLocalDate(), dateTo: todayLocalDate() });
  const [orchestrationPanel, setOrchestrationPanel] = useState<"orchestration" | "summary" | "dialogSummary" | "leadSummary" | "ai">("orchestration");
  const [orchestrationPrompt, setOrchestrationPrompt] = useState("Ты оркестратор режимов. По последним сообщениям пользователя, выбранным режимам и критериям режимов выбери самый подходящий режим и коротко объясни, почему переключение полезно.");
  const [dialogSummaryPrompt, setDialogSummaryPrompt] = useState("Сделай короткое резюме диалога и дай следующий практический шаг пользователю.");
  const [leadSummaryPrompt, setLeadSummaryPrompt] = useState("Ты помощник менеджера по продажам образовательного центра. По диалогу пользователя с ассистентом составь краткую выжимку для менеджера, 400-700 знаков, сплошным текстом без markdown и заголовков. Обязательно: кто человек и какая у него настоящая боль; на какой стадии осознанности он находится (по лестнице Ханта); метапрограммный профиль по речи (К/От, референция, масштаб); что уже пробовал; сомнения и опасения; каким следующим шагом менеджеру лучше выйти на контакт. Пиши только по фактам из диалога, ничего не выдумывай.");
  const [aiSettings, setAISettings] = useState({
    // Ollama fields removed 2026-05-28.
    summaryModel: "openai/gpt-4o-mini",
    summaryTemperature: "0.3",
    orchestrationModel: "openai/gpt-4o-mini",
    orchestrationTemperature: "0.1",
    orchestrationHistoryLimit: "12",
    leadSummaryModel: "openai/gpt-4o-mini",
    leadSummaryTemperature: "0.3",
    chatHistoryLimit: "10",
    chatMessageMaxChars: "30720",
    retryAttempts: "3",
    retryInitialDelayMs: "1500",
    queueEnabled: "1",
    queueIntervalMs: "1100",
    queueTimeoutMs: "60000",
    queueConcurrency: "1",
    attachmentDirectMaxBytes: "24576",
    attachmentMaxUploadBytes: "5242880",
    attachmentContextMaxChars: "24000",
    attachmentAnnotationModel: "openai/gpt-4o-mini",
    attachmentAnnotationPrompt: "Сожми вложенный файл для ответа в чате: сохрани факты, числа, термины, требования и явные вопросы. Не добавляй новых фактов.",
    providerVsegptEnabled: true,
    providerGeminiEnabled: true,
    providerAnthropicEnabled: true,
    providerVsegptConfigured: false,
    providerGeminiConfigured: false,
    providerAnthropicConfigured: false,
    providerGeminiDefaultModel: "gemini-3.1-flash-lite",
    providerAnthropicDefaultModel: "claude-haiku-4-5",
    providerVsegptDefaultModel: "google/gemini-3.1-flash-lite-thinking",
    // The real key never comes from the server, only a mask (****xxxx)
    // and a flag "overridden in the admin panel vs inherited from env".
    providerVsegptApiKeyMasked: "",
    providerGeminiApiKeyMasked: "",
    providerAnthropicApiKeyMasked: "",
    providerVsegptApiKeySetFromAdmin: false,
    providerGeminiApiKeySetFromAdmin: false,
    providerAnthropicApiKeySetFromAdmin: false,
    orchestrationProvider: "vsegpt",
    leadSummaryProvider: "vsegpt",
    summaryProvider: "vsegpt",
    attachmentAnnotationProvider: "vsegpt",
  });
  const [error, setError] = useState("");
  const [errorDebug, setErrorDebug] = useState<unknown>(null);
  const [notice, setNotice] = useState("");
  const [created, setCreated] = useState({ email: "", password: "", role: "user", status: "active", days: "30", modeIds: [] as number[] });
  const [grant, setGrant] = useState({ userId: "", email: "", days: "30", tariffId: "", modeIds: [] as number[], dailyMessageLimit: "50" });
  const [grantLimitManual, setGrantLimitManual] = useState(false);
  const [tariffForm, setTariffForm] = useState({ name: "", description: "", tariffType: "regular", monthlyPrice: "0", dailyMessageLimit: "50", limitType: "shared", groupId: "", availableForSubscription: true, modeIds: [] as number[], firstModeId: null as number | null });
  const [groupForm, setGroupForm] = useState({ name: "", description: "", availableForSubscription: true });
  const [selectedGroup, setSelectedGroup] = useState<any | null>(null);
  const [tariffSort, setTariffSort] = useState("created");
  const [tariffSection, setTariffSection] = useState<"list" | "groups" | "create">("list");
  const [newMode, setNewMode] = useState({ name: "", prompt: "", welcomeMessage: "", aiModel: "openai/gpt-4o-mini", modelTemperature: "0.7", criteria: "", orchestratorCheckInterval: "5", reminderCount: "0", hidden: false, audioEnabled: false });
  const [promoCreating, setPromoCreating] = useState(false);
  const [promoForm, setPromoForm] = useState({ bulkCount: "1", grantsType: "tariff", targetIds: [] as number[], firstModeId: 0, activeFrom: yesterdayLocalDate(), activeTo: monthAheadLocalDate(), durationDays: "30", maxUses: "1", dailyMessageLimit: "50", summaryLimit: "3", comment: "", purpose: "", temporaryAdmin: true });
  const [guardrail, setGuardrail] = useState({ text: defaultGuardrail, testModel: "", replaceAll: false, find: "", replacement: "" });
  const [modelStats, setModelStats] = useState<ModelStat[]>([]);
  const [exportFilters, setExportFilters] = useState({ modeIds: [] as number[], userIds: [] as number[], userSearch: "", modeSearch: "", promocodeIds: [] as number[], promocodeSearch: "", promptId: "", customPrompt: defaultSummaryPrompt, withSummary: true, roleFilter: "all", limit: "10000", dateFrom: monthBeforeLocalDate(), dateTo: todayLocalDate() });
  const [exportUsers, setExportUsers] = useState<UserRow[]>([]);
  const [exportModes, setExportModes] = useState<ModeRow[]>([]);
  const [exportPromocodes, setExportPromocodes] = useState<Promo[]>([]);
  const [exportOptionsLoaded, setExportOptionsLoaded] = useState(false);
  const [headerMobile, setHeaderMobile] = useState(false);
  const [broadcast, setBroadcast] = useState({ audience: "active_access", channel: "email", every: "once", message: "", promocodeIds: [] as number[], userIds: [] as number[] });
  const [newPrompt, setNewPrompt] = useState({ id: 0, name: "", prompt: defaultSummaryPrompt, isDefault: false });


  return {
    deleteModeHoldTimer, deleteModeHoldTriggered, tab, setTab, profile, setProfile, status, setStatus,
    adminStats, setAdminStats, statsSort, setStatsSort, exportSort, setExportSort, users, setUsers,
    userDetail, setUserDetail, grantUserDetail, setGrantUserDetail, userMeta, setUserMeta, modes,
    setModes, modeMeta, setModeMeta, modeDetail, setModeDetail, tariffs, setTariffs, groups, setGroups,
    selectedTariff, setSelectedTariff, selectedTariffIds, setSelectedTariffIds, tariffArchiveView,
    setTariffArchiveView, modePanel, setModePanel, promocodes, setPromocodes, lastCreatedPromos,
    setLastCreatedPromos, summaryPrompts, setSummaryPrompts, exportResult, setExportResult,
    exportSummaryPayload, setExportSummaryPayload, exportSummaryResult, setExportSummaryResult,
    exportSummaryHistory, setExportSummaryHistory, exportMeta, setExportMeta, userQ, setUserQ, modeQ,
    setModeQ, promoQ, setPromoQ, promoDateFrom, setPromoDateFrom, promoDateTo, setPromoDateTo, promoMeta, setPromoMeta, promoSection, setPromoSection, promoActivation, setPromoActivation,
    promoEdit, setPromoEdit, bulkPromoDeactivate, setBulkPromoDeactivate, orchestrationPanel, setOrchestrationPanel,
    orchestrationPrompt, setOrchestrationPrompt, dialogSummaryPrompt, setDialogSummaryPrompt,
    leadSummaryPrompt, setLeadSummaryPrompt,
    aiSettings, setAISettings, error, setError, errorDebug, setErrorDebug, notice, setNotice, created,
    setCreated, grant, setGrant, grantLimitManual, setGrantLimitManual, tariffForm, setTariffForm,
    groupForm, setGroupForm, selectedGroup, setSelectedGroup, tariffSort, setTariffSort, tariffSection,
    setTariffSection, newMode, setNewMode, promoCreating, setPromoCreating, promoForm, setPromoForm,
    guardrail, setGuardrail, modelStats, setModelStats, exportFilters, setExportFilters, exportUsers,
    setExportUsers, exportModes, setExportModes, exportPromocodes, setExportPromocodes,
    exportOptionsLoaded, setExportOptionsLoaded, headerMobile, setHeaderMobile, broadcast,
    setBroadcast, newPrompt, setNewPrompt,
  };
}
