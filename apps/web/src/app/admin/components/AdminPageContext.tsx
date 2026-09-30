"use client";

import React, { createContext, useContext } from "react";
import type { AppRouterInstance } from "next/dist/shared/lib/app-router-context.shared-runtime";
import type { CreatedPromo, ModeDetail, ModeRow, ModelStat, PageMeta, Promo, SortState, SummaryPrompt, Tab, Tariff, UserDetail, UserRow } from "../types";
import type { ApiError } from "@/lib/api";

type StateSetter<T> = React.Dispatch<React.SetStateAction<T>>;
export type MetricValue = string | number | boolean | null | undefined;
export type MetricRow = Record<string, MetricValue>;
type SortMap = Record<string, SortState>;

export type AdminProfile = { user?: UserRow; stats?: Record<string, MetricValue> };
export type AdminStatus = { counts?: Record<string, number> };
export type AdminStats = { totals?: Record<string, number>; daily?: MetricRow[]; byMode?: MetricRow[]; byUser?: MetricRow[]; byTariff?: MetricRow[] };
export type ExportMeta = { messageCount: number; sourceBytes: number; approxTokens: number };
export type CreatedUserForm = { email: string; password: string; role: string; status: string; days: string; modeIds: number[] };
export type GrantForm = { userId: string; email: string; days: string; tariffId: string; modeIds: number[]; dailyMessageLimit: string };
export type TariffForm = { name: string; description: string; tariffType: string; monthlyPrice: string; dailyMessageLimit: string; limitType: string; groupId: string; availableForSubscription: boolean; modeIds: number[]; firstModeId: number | null };
export type TariffGroup = { id?: number; name?: string; description?: string; availableForSubscription?: boolean };
export type GroupForm = { name: string; description: string; availableForSubscription: boolean };
export type NewModeForm = { name: string; prompt: string; welcomeMessage: string; aiModel: string; modelTemperature: string; criteria: string; orchestratorCheckInterval: string; reminderCount: string; hidden: boolean; audioEnabled: boolean };
export type PromoForm = { bulkCount: string; grantsType: string; targetIds: number[]; firstModeId: number; activeFrom: string; activeTo: string; durationDays: string; maxUses: string; dailyMessageLimit: string; summaryLimit: string; comment: string; purpose: string; temporaryAdmin: boolean };
export type GuardrailForm = { text: string; testModel: string; replaceAll: boolean; find: string; replacement: string };
export type ExportFilters = { modeIds: number[]; userIds: number[]; userSearch: string; modeSearch: string; promocodeIds: number[]; promocodeSearch: string; promptId: string; customPrompt: string; withSummary: boolean; roleFilter: string; limit: string; dateFrom: string; dateTo: string };
export type BroadcastForm = { audience: string; channel: string; every: string; message: string; promocodeIds: number[]; userIds: number[]; uiAudience?: string; uiSearch?: string };
export type PromoActivationForm = { id: number; activeTo: string; maxUses: string };
export type PromoSection = "active" | "exhausted" | "expired";
export type PromoEditForm = { id: number; grantsType: string; targetIds: number[]; firstModeId: number; activeTo: string; maxUses: string; updateScope: "new_only" | "all_activations" };
export type BulkPromoDeactivateForm = { dateFrom: string; dateTo: string };
export type AISettingsForm = {
  summaryModel: string;
  summaryTemperature: string;
  orchestrationModel: string;
  orchestrationTemperature: string;
  orchestrationHistoryLimit: string;
  leadSummaryModel: string;
  leadSummaryTemperature: string;
  chatHistoryLimit: string;
  chatMessageMaxChars: string;
  retryAttempts: string;
  retryInitialDelayMs: string;
  queueEnabled: string;
  queueIntervalMs: string;
  queueTimeoutMs: string;
  queueConcurrency: string;
  attachmentDirectMaxBytes: string;
  attachmentMaxUploadBytes: string;
  attachmentContextMaxChars: string;
  attachmentAnnotationModel: string;
  attachmentAnnotationPrompt: string;
  providerVsegptEnabled: boolean;
  providerGeminiEnabled: boolean;
  providerAnthropicEnabled: boolean;
  providerVsegptConfigured: boolean;
  providerGeminiConfigured: boolean;
  providerAnthropicConfigured: boolean;
  providerGeminiDefaultModel: string;
  providerAnthropicDefaultModel: string;
  providerVsegptDefaultModel: string;
  providerVsegptApiKeyMasked: string;
  providerGeminiApiKeyMasked: string;
  providerAnthropicApiKeyMasked: string;
  providerVsegptApiKeySetFromAdmin: boolean;
  providerGeminiApiKeySetFromAdmin: boolean;
  providerAnthropicApiKeySetFromAdmin: boolean;
  // Provider for service AI mechanics (not tied to a specific mode).
  orchestrationProvider: string;
  leadSummaryProvider: string;
  summaryProvider: string;
  attachmentAnnotationProvider: string;
};

export type AdminPageContextValue = {
  tab: Tab; setTab: StateSetter<Tab>; profile: AdminProfile | null; setProfile: StateSetter<AdminProfile | null>; status: AdminStatus | null; setStatus: StateSetter<AdminStatus | null>; adminStats: AdminStats | null; setAdminStats: StateSetter<AdminStats | null>;
  statsSort: SortMap; setStatsSort: StateSetter<SortMap>; exportSort: SortMap; setExportSort: StateSetter<SortMap>; users: UserRow[]; setUsers: StateSetter<UserRow[]>; userDetail: UserDetail | null; setUserDetail: StateSetter<UserDetail | null>; grantUserDetail: UserDetail | null; setGrantUserDetail: StateSetter<UserDetail | null>; userMeta: PageMeta; setUserMeta: StateSetter<PageMeta>;
  modes: ModeRow[]; setModes: StateSetter<ModeRow[]>; modeMeta: PageMeta; setModeMeta: StateSetter<PageMeta>; modeDetail: ModeDetail | null; setModeDetail: StateSetter<ModeDetail | null>; tariffs: Tariff[]; setTariffs: StateSetter<Tariff[]>; groups: TariffGroup[]; setGroups: StateSetter<TariffGroup[]>; selectedTariff: Tariff | null; setSelectedTariff: StateSetter<Tariff | null>; selectedTariffIds: number[]; setSelectedTariffIds: StateSetter<number[]>;
  tariffArchiveView: "active" | "archive"; setTariffArchiveView: StateSetter<"active" | "archive">; modePanel: "edit" | "create" | "guardrail"; setModePanel: StateSetter<"edit" | "create" | "guardrail">; promocodes: Promo[]; setPromocodes: StateSetter<Promo[]>; lastCreatedPromos: CreatedPromo[]; setLastCreatedPromos: StateSetter<CreatedPromo[]>; summaryPrompts: SummaryPrompt[]; setSummaryPrompts: StateSetter<SummaryPrompt[]>;
  exportResult: MetricRow[]; setExportResult: StateSetter<MetricRow[]>; exportSummaryPayload: string; setExportSummaryPayload: StateSetter<string>; exportSummaryResult: string; setExportSummaryResult: StateSetter<string>; exportSummaryHistory: MetricRow[]; setExportSummaryHistory: StateSetter<MetricRow[]>; exportMeta: ExportMeta; setExportMeta: StateSetter<ExportMeta>;
  userQ: string; setUserQ: StateSetter<string>; modeQ: string; setModeQ: StateSetter<string>; promoQ: string; setPromoQ: StateSetter<string>; promoDateFrom: string; setPromoDateFrom: StateSetter<string>; promoDateTo: string; setPromoDateTo: StateSetter<string>; promoMeta: PageMeta; setPromoMeta: StateSetter<PageMeta>; promoSection: PromoSection; setPromoSection: StateSetter<PromoSection>; promoActivation: PromoActivationForm | null; setPromoActivation: StateSetter<PromoActivationForm | null>; promoEdit: PromoEditForm | null; setPromoEdit: StateSetter<PromoEditForm | null>; bulkPromoDeactivate: BulkPromoDeactivateForm; setBulkPromoDeactivate: StateSetter<BulkPromoDeactivateForm>;
  orchestrationPanel: "orchestration" | "summary" | "dialogSummary" | "leadSummary" | "ai"; setOrchestrationPanel: StateSetter<"orchestration" | "summary" | "dialogSummary" | "leadSummary" | "ai">; orchestrationPrompt: string; setOrchestrationPrompt: StateSetter<string>; dialogSummaryPrompt: string; setDialogSummaryPrompt: StateSetter<string>; leadSummaryPrompt: string; setLeadSummaryPrompt: StateSetter<string>; aiSettings: AISettingsForm; setAISettings: StateSetter<AISettingsForm>;
  error: string; setError: StateSetter<string>; errorDebug: unknown; setErrorDebug: StateSetter<unknown>; notice: string; setNotice: StateSetter<string>; created: CreatedUserForm; setCreated: StateSetter<CreatedUserForm>; grant: GrantForm; setGrant: StateSetter<GrantForm>; grantLimitManual: boolean; setGrantLimitManual: StateSetter<boolean>;
  tariffForm: TariffForm; setTariffForm: StateSetter<TariffForm>; groupForm: GroupForm; setGroupForm: StateSetter<GroupForm>; selectedGroup: TariffGroup | null; setSelectedGroup: StateSetter<TariffGroup | null>; tariffSort: string; setTariffSort: StateSetter<string>; tariffSection: "list" | "groups" | "create"; setTariffSection: StateSetter<"list" | "groups" | "create">; newMode: NewModeForm; setNewMode: StateSetter<NewModeForm>;
  promoCreating: boolean; promoForm: PromoForm; setPromoForm: StateSetter<PromoForm>; guardrail: GuardrailForm; setGuardrail: StateSetter<GuardrailForm>; modelStats: ModelStat[]; setModelStats: StateSetter<ModelStat[]>; exportFilters: ExportFilters; setExportFilters: StateSetter<ExportFilters>; exportUsers: UserRow[]; setExportUsers: StateSetter<UserRow[]>; exportModes: ModeRow[]; setExportModes: StateSetter<ModeRow[]>; exportPromocodes: Promo[]; setExportPromocodes: StateSetter<Promo[]>; exportOptionsLoaded: boolean; setExportOptionsLoaded: StateSetter<boolean>; broadcast: BroadcastForm; setBroadcast: StateSetter<BroadcastForm>; newPrompt: SummaryPrompt; setNewPrompt: StateSetter<SummaryPrompt>;
  router: AppRouterInstance; deleteModeHoldTimer: React.MutableRefObject<ReturnType<typeof setTimeout> | null>; deleteModeHoldTriggered: React.MutableRefObject<boolean>;
  userPage: number; totalPages: number; modePage: number; modeTotalPages: number; activeTariffs: Tariff[]; archivedTariffs: Tariff[]; visibleTariffs: Tariff[]; activePromos: Promo[]; exhaustedPromos: Promo[]; expiredPromos: Promo[]; filteredUsersBase: UserRow[]; filteredUsers: UserRow[]; filteredPromosBase: Promo[]; filteredPromos: Promo[]; promoModeSet: Set<number> | null; filteredExportModesBase: ModeRow[]; filteredExportModes: ModeRow[]; activeGrantModeIds: number[]; grantModeOptions: ModeRow[];
  nextSort: (current: SortState, key: string) => SortState; sortValue: (row: Record<string, unknown>, key: string) => MetricValue; sortButton: (label: string, key: string, current: SortState, onClick: () => void) => React.ReactElement; toggleExportSort: (scope: string, key: string) => void; toggleStatsSort: (title: string, key: string) => void; selectedPromoModeSet: () => Set<number> | null;
  showNotice: (message: string) => void; handleError: (e: unknown, fallback: string) => boolean; loadProfile: () => Promise<AdminProfile | null>; loadSystem: () => Promise<void>; loadAdminStats: () => Promise<void>; loadUsers: (offset?: number, limit?: number) => Promise<void>; loadModes: (offset?: number, limit?: number) => Promise<void>; loadTariffs: (sort?: string) => Promise<void>; loadGroups: () => Promise<void>; loadPromocodes: (offset?: number, limit?: number) => Promise<void>; loadExportOptions: (force?: boolean) => Promise<void>; loadSummaryPrompts: () => Promise<void>; loadModelStats: () => Promise<void>; loadGuardrail: () => Promise<void>; loadOrchestrationPrompt: () => Promise<void>; loadDialogSummaryPrompt: () => Promise<void>; loadAISettings: () => Promise<void>;
  setModeSelection: (scope: "created" | "grant" | "tariff" | "promo", ids: number[]) => void; toggleModeId: (scope: "created" | "grant" | "tariff" | "promo", id: number) => void; modeSelectionToolbar: (scope: "created" | "grant" | "promo") => React.ReactElement; modeCheckboxes: (scope: "created" | "grant" | "tariff" | "promo", ids: number[], showChains?: boolean) => React.ReactElement;
  createUser: (e: React.FormEvent) => Promise<void>; selectGrantUser: (user: UserRow) => Promise<void>; syncSelectedUserAccess: () => Promise<boolean>; resetSelectedUserModeLimits: () => Promise<void>; grantAccess: (e: React.FormEvent) => Promise<void>; patchUser: (user: UserRow, patch: Partial<UserRow>) => Promise<void>; openUser: (user: UserRow) => Promise<void>; openMode: (mode: ModeRow) => Promise<void>; saveMode: (e: React.FormEvent) => Promise<void>; copyMode: () => Promise<void>; createMode: (e: React.FormEvent) => Promise<void>; testModeModel: () => Promise<void>; detachModeFromPaidTariffs: () => Promise<void>; cancelDeleteModeHold: () => void; startDeleteModeHold: () => void; deleteModeFromDatabase: () => Promise<void>; applyGuardrail: () => Promise<void>; testGuardrailModel: () => Promise<void>;
  createTariff: (e: React.FormEvent) => Promise<void>; saveTariff: (e: React.FormEvent) => Promise<void>; applyTariffModesAI: (provider: string, model: string) => Promise<void>; createGroup: (e: React.FormEvent) => Promise<void>; saveGroup: (e: React.FormEvent) => Promise<void>; createPromocodes: (e: React.FormEvent) => Promise<void>; savePromocodeEdit: () => Promise<void>; restoreTariff: (tariff: Tariff) => Promise<void>; deleteTariff: (id: number, permanent?: boolean) => Promise<void>; deleteSelectedTariffs: () => Promise<void>; deleteGroup: (id: number, name: string) => Promise<void>; deactivatePromocode: (id: number) => Promise<void>; activatePromocode: () => Promise<void>; bulkDeactivatePromocodes: () => Promise<void>;
  saveOrchestrationPrompt: () => Promise<void>; saveDialogSummaryPrompt: () => Promise<void>; saveLeadSummaryPrompt: () => Promise<void>; saveAISettings: (newApiKeys?: { vsegpt?: string; gemini?: string; anthropic?: string }) => Promise<void>; exportMessages: () => Promise<void>; summarizeExport: () => Promise<void>; createSummaryPrompt: (e: React.FormEvent) => Promise<void>; deleteSummaryPrompt: (id: number) => Promise<void>; editSummaryPrompt: (prompt: SummaryPrompt) => void; sendBroadcast: (e: React.FormEvent) => Promise<void>; createdPromosTable: (items?: CreatedPromo[]) => string; downloadText: (filename: string, text: string) => void; copyCreatedPromos: () => Promise<void>; downloadCreatedPromos: () => void; downloadTxt: () => void; copy: (text: string) => Promise<void>; logout: () => Promise<void>;
  roles: readonly string[]; roleDescriptions: Record<string, string>; tabs: Array<{ id: Tab; label: string }>; formatDate: (value?: string | null) => string; promoStateLabel: (promo: Promo) => string; QRImage: typeof import("./QRImage").QRImage; MessageContent: React.ComponentType<{ content: string }>; statuses: readonly string[]; pageSizes: readonly number[]; statusHelp: (status: string) => string; passwordScore: (password: string) => number; selectedNumberValues: (options: HTMLCollectionOf<HTMLOptionElement>) => number[]; toggleNumberSelection: (ids: number[], id: number) => number[]; ExportLimitSlider: React.ComponentType<{ value: string; onCommit: (value: string) => void }>;
  grantLimitSliderMax: number; grantLimitFromSlider: (value: string) => number; grantSliderFromLimit: (value: string) => string; normalizeGrantLimit: (value: string) => string; sortedRows: <T extends Record<string, unknown>>(rows: T[], sort: SortState) => T[]; PromoColumn: typeof import("./PromoColumn").PromoColumn; modeNamesByIds: (modes: ModeRow[], ids?: number[]) => string; monthAheadLocalDate: () => string; ApiError: typeof ApiError;
};

export const AdminPageContext = createContext<AdminPageContextValue | null>(null);

export function useAdminPageContext(): AdminPageContextValue {
  const value = useContext(AdminPageContext);
  if (!value) throw new Error("AdminPageContext is missing");
  return value;
}
