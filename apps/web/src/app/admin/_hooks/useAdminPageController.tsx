"use client";

import React, { Dispatch, SetStateAction, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { MessageContent } from "@/components/MessageContent";
import { ApiError, apiFetch } from "@/lib/api";
import { QRImage } from "../components/QRImage";
import { PromoColumn } from "../components/PromoColumn";
import type {
  Tab, UserRow, UserAccessMode, UserDetail, ModeRow, ModeDetail,
  Tariff, Promo, CreatedPromo, SummaryPrompt, PageMeta, ModelStat, SortState,
} from "../types";
import {
  tabs, roles, roleDescriptions, statuses, pageSizes, defaultSummaryPrompt, defaultGuardrail,
  grantLimitTail, grantLimitSliderMax,
} from "../constants";
import {
  grantLimitFromSlider, grantSliderFromLimit, normalizeGrantLimit, formatDate, statusHelp,
  passwordScore, selectedNumberValues, toggleNumberSelection, toLocalDateInput, todayLocalDate,
  yesterdayLocalDate, weekAgoLocalDate, monthAheadLocalDate, monthBeforeLocalDate, exportLimitLabel,
  modeNamesByIds, fullClientUrl, promoIsExhaustedMultiUse, promoStateLabel,
} from "../utils";
import { ExportLimitSlider } from "../components/ExportLimitSlider";
import { createAdminCommonActions } from "./adminCommonActions";
import { createAdminLoaders } from "./adminLoaders";
import { createAdminBillingActions } from "./adminBillingActions";
import { createAdminExportActions } from "./adminExportActions";
import { useAdminControllerState } from "./useAdminControllerState";
import { useAdminModeUserActions } from "./useAdminModeUserActions";

export function useAdminPageController() {
  const router = useRouter();

  useEffect(() => {
    // The admin panel is a document page, so we reset a possible scroll
    // lock left by full-screen mobile screens (e.g. the chat).
    document.documentElement.style.overflow = "";
    document.body.style.overflow = "";
    document.body.style.overscrollBehaviorY = "";
  }, []);
  const state = useAdminControllerState();
  const {
    deleteModeHoldTimer, deleteModeHoldTriggered, tab, setTab, profile, setProfile, status, setStatus,
    adminStats, setAdminStats, statsSort, setStatsSort, exportSort, setExportSort, users, setUsers,
    userDetail, setUserDetail, grantUserDetail, setGrantUserDetail, userMeta, setUserMeta, modes,
    setModes, modeMeta, setModeMeta, modeDetail, setModeDetail, tariffs, setTariffs, groups, setGroups,
    selectedTariff, setSelectedTariff, selectedTariffIds, setSelectedTariffIds, tariffArchiveView,
    setTariffArchiveView, modePanel, setModePanel, promocodes, setPromocodes, lastCreatedPromos,
    setLastCreatedPromos, summaryPrompts, setSummaryPrompts, exportResult, setExportResult,
    exportSummaryPayload, setExportSummaryPayload, exportSummaryResult, setExportSummaryResult,
    exportSummaryHistory, setExportSummaryHistory, exportMeta, setExportMeta, userQ, setUserQ, modeQ,
    setModeQ, promoQ, setPromoQ, promoDateFrom, setPromoDateFrom, promoDateTo, setPromoDateTo, promoMeta: promoMetaState, setPromoMeta, promoSection, setPromoSection, promoActivation, setPromoActivation,
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
  } = state;

  const commonActions = createAdminCommonActions({
    router, setError, setErrorDebug, setNotice, setExportSort, setStatsSort,
    exportPromocodes, exportFilters, tabs,
  });
  const { nextSort, sortValue, sortedRows, sortButton, toggleExportSort, toggleStatsSort, selectedPromoModeSet, showNotice, handleError, adminTabsForRole, canUseAdminPage } = commonActions;

  const userPage = Math.floor(userMeta.offset / userMeta.limit) + 1;
  const totalPages = useMemo(() => Math.max(1, Math.ceil(userMeta.total / userMeta.limit)), [userMeta]);
  const modePage = Math.floor(modeMeta.offset / modeMeta.limit) + 1;
  const modeTotalPages = useMemo(() => Math.max(1, Math.ceil(modeMeta.total / modeMeta.limit)), [modeMeta]);
  const activeTariffs = tariffs.filter((t) => !t.archivedAt && t.availableForSubscription);
  const archivedTariffs = tariffs.filter((t) => t.archivedAt || !t.availableForSubscription);
  const visibleTariffs = tariffArchiveView === "active" ? activeTariffs : archivedTariffs;
  const activePromos = promocodes.filter((p) => p.active);
  const exhaustedPromos = promocodes.filter((p) => promoIsExhaustedMultiUse(p));
  const expiredPromos = promocodes.filter((p) => !p.active && !promoIsExhaustedMultiUse(p));

  const filteredUsersBase = exportUsers.filter((u) =>
    `${u.email} ${u.telegramUsername} ${u.id} ${u.createdAt}`.toLowerCase().includes(exportFilters.userSearch.toLowerCase())
  );
  const filteredUsers = sortedRows(filteredUsersBase, exportSort.users || null);
  const filteredPromosBase = exportPromocodes.filter((p) =>
    `${p.id} ${p.code} ${p.comment || ""} ${p.purpose || ""} ${p.grantsType || ""} ${p.targetId || ""} ${p.maxUses || ""} ${p.usedCount || ""} ${p.activeFrom || ""} ${p.activeTo || ""} ${p.lastUsedAt || ""} ${promoStateLabel(p)}`.toLowerCase().includes(exportFilters.promocodeSearch.toLowerCase())
  );
  const filteredPromos = sortedRows(filteredPromosBase, exportSort.promos || null);
  const promoModeSet = selectedPromoModeSet();
  const filteredExportModesBase = exportModes.filter(
    (m) => `${m.name} ${m.id}`.toLowerCase().includes(exportFilters.modeSearch.toLowerCase()) && (!promoModeSet || promoModeSet.has(m.id))
  );
  const filteredExportModes = sortedRows(filteredExportModesBase, exportSort.modes || null);

  useEffect(() => {
    const allowed = selectedPromoModeSet();
    if (!allowed) return;
    setExportFilters((value) => ({ ...value, modeIds: value.modeIds.filter((id) => allowed.has(id)) }));
  }, [exportFilters.promocodeIds, exportPromocodes]);

  const loaders = createAdminLoaders({
    userMeta, modeMeta, userQ, modeQ, tariffSort, promoQ, promoDateFrom, promoDateTo, promoMeta: promoMetaState, setPromoMeta, exportOptionsLoaded, setProfile,
    setStatus, setAdminStats, setUsers, setUserMeta, setModes, setModeMeta, setTariffs,
    setGroups, setPromocodes, setExportUsers, setExportModes, setExportPromocodes,
    setExportOptionsLoaded, setSummaryPrompts, setExportFilters, setModelStats,
    setOrchestrationPrompt, setDialogSummaryPrompt, setLeadSummaryPrompt, setAISettings, setGuardrail, handleError,
  });
  const { loadProfile, loadSystem, loadAdminStats, loadUsers, loadModes, loadTariffs, loadGroups, loadPromocodes, loadExportOptions, loadSummaryPrompts, loadModelStats, loadGuardrail, loadOrchestrationPrompt, loadDialogSummaryPrompt, loadLeadSummaryPrompt, loadAISettings } = loaders;

  useEffect(() => {
    let cancelled = false;

    const bootstrapAdmin = async () => {
      const loadedProfile = await loadProfile();
      if (cancelled) return;

      const role = loadedProfile?.user?.role;
      if (!canUseAdminPage(role)) {
        router.replace(role ? "/profile" : "/login");
        return;
      }

      const loadFullAdmin = role === "owner" || role === "admin";
      const tasks: Array<Promise<unknown>> = [];

      if (loadFullAdmin || role === "support" || role === "billing_admin") {
        tasks.push(loadSystem());
      }
      if (loadFullAdmin || role === "support") {
        tasks.push(loadUsers(0));
      }
      if (loadFullAdmin || role === "content_admin") {
        tasks.push(loadModes(0), loadSummaryPrompts(), loadModelStats(), loadGuardrail(), loadOrchestrationPrompt(), loadAISettings(), loadDialogSummaryPrompt(), loadLeadSummaryPrompt());
      }
      if (loadFullAdmin || role === "billing_admin") {
        tasks.push(loadTariffs(), loadGroups());
      }
      if (loadFullAdmin) {
        tasks.push(loadPromocodes());
      }
      if (loadFullAdmin || role === "support" || role === "billing_admin") {
        tasks.push(loadExportOptions());
      }

      await Promise.allSettled(tasks);
    };

    void bootstrapAdmin();
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    const check = () => setHeaderMobile(window.innerWidth <= 900);
    check();
    window.addEventListener("resize", check);
    return () => window.removeEventListener("resize", check);
  }, []);

  const modeUserActions = useAdminModeUserActions({
    created, setCreated, grant, setGrant, tariffForm, setTariffForm, promoForm, setPromoForm,
    grantUserDetail, setGrantUserDetail, exportModes, modes, setError, showNotice, loadUsers,
    handleError, setUserDetail,
  });
  const { activeGrantModeIds, grantModeOptions, setModeSelection, toggleModeId, modeSelectionToolbar, modeCheckboxes, createUser, selectGrantUser, syncSelectedUserAccess, resetSelectedUserModeLimits, grantAccess, patchUser, openUser } = modeUserActions;

  const billingActions = createAdminBillingActions({
    modeDetail, setModeDetail, newMode, setNewMode, deleteModeHoldTimer, deleteModeHoldTriggered,
    guardrail, modes, tariffForm, selectedTariff, groupForm, selectedGroup, promoCreating, promoForm,
    setPromoCreating, setError, setLastCreatedPromos, setPromoForm, setNotice, tariffs,
    setSelectedTariff, setSelectedTariffIds, selectedTariffIds, setSelectedGroup, promoActivation,
    setPromoActivation, promoEdit, setPromoEdit, bulkPromoDeactivate, orchestrationPrompt, dialogSummaryPrompt, leadSummaryPrompt, aiSettings,
    showNotice, handleError, loadModes, loadExportOptions, loadModelStats, loadTariffs, loadGroups, loadPromocodes,
  });
  const { openMode, saveMode, createMode, testModeModel, detachModeFromPaidTariffs, cancelDeleteModeHold, startDeleteModeHold, deleteModeFromDatabase, applyGuardrail, testGuardrailModel, createTariff, saveTariff, applyTariffModesAI, createGroup, saveGroup, createPromocodes, savePromocodeEdit, restoreTariff, deleteTariff, deleteSelectedTariffs, deleteGroup, deactivatePromocode, activatePromocode, bulkDeactivatePromocodes, saveOrchestrationPrompt, saveDialogSummaryPrompt, saveAISettings } = billingActions;

  const exportActions = createAdminExportActions({
    exportFilters, exportModes, setExportResult, setExportSummaryPayload, setExportMeta, setExportSummaryResult,
    setExportSummaryHistory, newPrompt, setNewPrompt, loadSummaryPrompts, broadcast, lastCreatedPromos,
    exportSummaryPayload, exportResult, setError, setErrorDebug, showNotice, handleError, router,
  });
  const { exportMessages, summarizeExport, createSummaryPrompt, deleteSummaryPrompt, editSummaryPrompt, sendBroadcast, createdPromosTable, downloadText, copyCreatedPromos, downloadCreatedPromos, downloadTxt, copy, logout } = exportActions;

  const roleTabs = adminTabsForRole(profile?.user?.role);
  const visibleTabs = roleTabs.length ? tabs.filter((item) => roleTabs.includes(item.id)) : [{ id: "profile" as Tab, label: "Профиль" }];
  const visibleTabIds = new Set(visibleTabs.map((item) => item.id));
  function selectTab(nextTab: Tab) {
    if (!visibleTabIds.has(nextTab)) return;
    setTab(nextTab);
    setError("");
    showNotice("");
    if (nextTab === "stats" && !adminStats) void loadAdminStats();
    if (nextTab === "exports") void loadExportOptions(true);
  }

  const adminPageContext = {
    tab, setTab, profile, setProfile, status, setStatus, adminStats, setAdminStats,
    statsSort, setStatsSort, exportSort, setExportSort, users, setUsers, userDetail, setUserDetail,
    grantUserDetail, setGrantUserDetail, userMeta, setUserMeta, modes, setModes, modeMeta, setModeMeta,
    modeDetail, setModeDetail, tariffs, setTariffs, groups, setGroups, selectedTariff, setSelectedTariff,
    selectedTariffIds, setSelectedTariffIds, tariffArchiveView, setTariffArchiveView, modePanel, setModePanel,
    promocodes, setPromocodes, lastCreatedPromos, setLastCreatedPromos, summaryPrompts, setSummaryPrompts,
    exportResult, setExportResult, exportSummaryPayload, setExportSummaryPayload, exportSummaryResult, setExportSummaryResult,
    exportSummaryHistory, setExportSummaryHistory, exportMeta, setExportMeta, userQ, setUserQ, modeQ, setModeQ,
    promoQ, setPromoQ, promoDateFrom, setPromoDateFrom, promoDateTo, setPromoDateTo, promoMeta: promoMetaState, setPromoMeta, promoSection, setPromoSection, promoActivation, setPromoActivation,
    promoEdit, setPromoEdit,
    bulkPromoDeactivate, setBulkPromoDeactivate, orchestrationPanel, setOrchestrationPanel,
    orchestrationPrompt, setOrchestrationPrompt, dialogSummaryPrompt, setDialogSummaryPrompt,
    leadSummaryPrompt, setLeadSummaryPrompt,
    aiSettings, setAISettings, error, setError, errorDebug, setErrorDebug, notice, setNotice,
    created, setCreated, grant, setGrant, grantLimitManual, setGrantLimitManual,
    tariffForm, setTariffForm, groupForm, setGroupForm, selectedGroup, setSelectedGroup,
    tariffSort, setTariffSort, tariffSection, setTariffSection, newMode, setNewMode,
    promoCreating, promoForm, setPromoForm, guardrail, setGuardrail, modelStats, setModelStats,
    exportFilters, setExportFilters, exportUsers, setExportUsers, exportModes, setExportModes,
    exportPromocodes, setExportPromocodes, exportOptionsLoaded, setExportOptionsLoaded,
    broadcast, setBroadcast, newPrompt, setNewPrompt, router, deleteModeHoldTimer, deleteModeHoldTriggered,
    userPage, totalPages, modePage, modeTotalPages, activeTariffs, archivedTariffs, visibleTariffs,
    activePromos, exhaustedPromos, expiredPromos, filteredUsersBase, filteredUsers, filteredPromosBase, filteredPromos,
    promoModeSet, filteredExportModesBase, filteredExportModes,
    ...commonActions, ...loaders, ...modeUserActions, ...billingActions, ...exportActions, roles, roleDescriptions, tabs: visibleTabs, formatDate, promoStateLabel, QRImage, MessageContent,
    statuses, pageSizes, statusHelp, passwordScore, selectedNumberValues, toggleNumberSelection,
    ExportLimitSlider, grantLimitSliderMax, grantLimitFromSlider, grantSliderFromLimit, normalizeGrantLimit,
    sortedRows, PromoColumn, modeNamesByIds, monthAheadLocalDate, ApiError,
  };

  return {
    adminPageContext, headerMobile, visibleTabs, tab, selectTab,
    error, errorDebug, notice, logout,
  };
}
