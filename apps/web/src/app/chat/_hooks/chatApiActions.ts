"use client";

import { ApiError } from "@/lib/api";
import type { Message, StartPayload, HistoryPayload } from "../types";
import type { ChatApiActionsContext } from "./chatActionTypes";
import {
  apiBase, cleanOutgoingMessage, modeHasMessages, modeSwitchTraceContent, prependUniqueMessages, replaceTrailingModeSwitchTrace, selectableModeIds, syncModesWithGlobalUsage,
} from "../utils";
import {
  readPendingSelectedModeIds, readStoredSelectedModeIds, writeHistory,
} from "../storage";

export function createChatApiActions(ctx: ChatApiActionsContext) {
  const {
    modes, quota, modeId, modeName, input, attachments, chatMessageMaxChars, dialogId, selectedModeIds, beforeId, responseMode,
    dialogCompleted, dialogAccessRequired, dailyLimitExhausted, sending, completing, startingAfterSummary,
    modeSelectionLoadedRef, loadingMoreRef, setLoading, setError, setErrorDebug, setUserRole, setUserEmail, setMaxLinked, setMaxBotLink,
    setResponseMode, setModes, setSelectedModeIds, setPromoModeIds, setQuota, setChatMessageMaxChars, setModeId, setModeName,
    setDialogId, setMessages, setBeforeId, setSummaryHistoryVisible, setDialogAccessRequired,
    setDialogCompleted, setInput, setAttachments, setOrchestrationNotice, setLockedModeNotice, setSending, setCompleting,
    setStartingAfterSummary, scrollChatToBottom,
  } = ctx;

  async function start() {
    setLoading(true); setError(null); setErrorDebug(null);
    try {
      const res = await fetch(`${apiBase}/api/chat/start`, { method: "POST", credentials: "include" });
      if (!res.ok) { if (res.status === 403) { window.location.replace("/access?force=promo"); return; } throw new Error(`HTTP ${res.status}`); }
      const json: StartPayload = await res.json();
      if (!json.ok) throw new Error(json.error || "Не удалось запустить чат");
      if (typeof json.chatMessageMaxChars === "number" && Number.isFinite(json.chatMessageMaxChars) && json.chatMessageMaxChars > 0) {
        setChatMessageMaxChars(Math.floor(json.chatMessageMaxChars));
      }
      const role = json.role || ""; setUserRole(role);
      setUserEmail(json.email || "");
      setMaxLinked(json.maxLinked ?? false);
      setMaxBotLink(json.maxBotLink ?? "");
      const canChooseMode = role === "admin" || role === "owner" || role === "tester";
      if (canChooseMode) { const storedMode = localStorage.getItem("chat_response_mode"); setResponseMode(storedMode === "live" ? "live" : "test"); } else { setResponseMode("live"); localStorage.removeItem("chat_response_mode"); }
      const availableModes = json.quota ? syncModesWithGlobalUsage(json.modes || [], json.quota) : (json.modes || []);
      const allModeIds = selectableModeIds(availableModes);
      const promoModeIdsFromStart = readPendingSelectedModeIds(availableModes)?.filter(id => allModeIds.includes(id));
      const storedModeSelection = readStoredSelectedModeIds(availableModes);
      const storedModeIds = storedModeSelection.ids.filter(id => allModeIds.includes(id));
      const defaultSelectedIds = promoModeIdsFromStart?.length
        ? promoModeIdsFromStart
        : !canChooseMode && allModeIds.length > 1 && storedModeIds.length < 2
          ? allModeIds
        : storedModeSelection.exists
          ? storedModeIds
          : (json.currentModeId ? [json.currentModeId].filter(id => allModeIds.includes(id)) : []);
      const currentModeIsSelected = Boolean(json.currentModeId && defaultSelectedIds.includes(json.currentModeId));
      modeSelectionLoadedRef.current = false;
		setModes(availableModes);
		setSelectedModeIds(defaultSelectedIds); setQuota(json.quota || null);
      if (promoModeIdsFromStart?.length) { setPromoModeIds(promoModeIdsFromStart); await openMode(promoModeIdsFromStart[0], "", true); setSelectedModeIds(promoModeIdsFromStart); modeSelectionLoadedRef.current = true; return; }
      if (defaultSelectedIds.length === 0) { await openBaseMode(); modeSelectionLoadedRef.current = true; return; }
      if (!currentModeIsSelected && defaultSelectedIds[0]) { await openMode(defaultSelectedIds[0]); modeSelectionLoadedRef.current = true; return; }
      if (json.currentModeId) {
        const currentMode = availableModes.find(m => m.id === json.currentModeId);
        // #5: if the current mode's quota is exhausted, switch to the first
        // available one among the selected. Otherwise the user comes in and immediately sees the "limit"
        // instead of a normal chat with another mode.
        const currentExhausted = currentMode ? !modeHasMessages(currentMode) : false;
        if (currentExhausted) {
          const fallback = defaultSelectedIds
            .map(id => availableModes.find(m => m.id === id))
            .find(m => m && modeHasMessages(m));
          if (fallback) {
            await openMode(fallback.id);
            modeSelectionLoadedRef.current = true;
            return;
          }
          // All selected ones are exhausted: fall back to the base mode (it has no quota).
          await openBaseMode();
          modeSelectionLoadedRef.current = true;
          return;
        }
        setModeId(json.currentModeId);
        if (currentMode) setModeName(currentMode.name);
      }
      if (json.currentDialogId) {
        setDialogId(json.currentDialogId);
        if (json.messages) {
          setSummaryHistoryVisible(true);
          setDialogAccessRequired(false);
          setDialogCompleted(false);
          setMessages(json.messages);
          setBeforeId(json.messages[0]?.id ?? null);
          scrollChatToBottom("auto");
        } else {
          await loadHistory(json.currentDialogId, false, availableModes, defaultSelectedIds);
        }
      }
      modeSelectionLoadedRef.current = true;
    } catch (e) { setError(e instanceof Error ? e.message : "Ошибка запуска чата"); }
    finally { setLoading(false); }
  }

  async function loadHistory(targetDialogId: number, more = false, knownModes = modes, defaultSelectedModeIds: number[] = selectedModeIds) {
    try {
      const params = new URLSearchParams({ dialogId: String(targetDialogId), limit: "20" });
      if (more && beforeId) params.set("beforeId", String(beforeId));
      const res = await fetch(`${apiBase}/api/chat/history?${params.toString()}`, { credentials: "include" });
      const json: HistoryPayload = await res.json().catch(() => ({}));
      if (!res.ok) { if (res.status === 403) { setDialogAccessRequired(true); setInput(""); throw new Error("Доступ к режиму диалога закончился. Продлите доступ, чтобы продолжить."); } if (res.status === 404) throw new Error("Диалог не найден или больше недоступен."); throw new Error(json.error || `HTTP ${res.status}`); }
      if (!json.ok) throw new Error(json.error || "Ошибка загрузки истории");
      const incoming: Message[] = json.messages || [];
      if (!more) {
        setSummaryHistoryVisible(true);
        if (json.dialogId) setDialogId(json.dialogId);
        const needsAccess = json.accessRequired === true || json.accessActive === false;
        setDialogAccessRequired(needsAccess); if (needsAccess) setInput("");
        if (typeof json.modeId === "number") { setModeId(json.modeId); setModeName(json.modeName || knownModes.find(m => m.id === json.modeId)?.name || ""); setSelectedModeIds(prev => { const base = prev.length ? prev : defaultSelectedModeIds; return base.length ? base.includes(json.modeId as number) ? base : [json.modeId as number, ...base] : [json.modeId as number]; }); }
        setDialogCompleted(false);
      }
      if (incoming.length > 0) setBeforeId(incoming[0].id);
      loadingMoreRef.current = more;
      setMessages(prev => more ? prependUniqueMessages(incoming, prev) : incoming);
      if (!more) scrollChatToBottom("auto"); return true;
    } catch (e) { setError(e instanceof Error ? e.message : "Ошибка загрузки истории"); return false; }
  }

  async function openMode(nextModeId: number, preservedInput = "", newDialog = false) {
    setModeId(nextModeId); setError(null); setErrorDebug(null); setOrchestrationNotice(""); setLockedModeNotice("");
    try {
      const res = await fetch(`${apiBase}/api/chat/select-mode`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ modeId: nextModeId, newDialog }) });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) {
        if (res.status === 429) {
          const updatedQuota = json.quota || quota;
          setQuota(updatedQuota);
          if (updatedQuota) setModes(prev => syncModesWithGlobalUsage(prev, updatedQuota));
          setInput(preservedInput);
          return;
        }
        throw new ApiError(json.error || `HTTP ${res.status}`, res.status, json);
      }
      if (!json.ok) throw new Error(json.error || "Не удалось открыть режим");
      setDialogId(json.dialogId); setDialogAccessRequired(false); setModeName(json.modeName);
      setQuota(prevQuota => json.quota ?? prevQuota);
      if (json.quota) setModes(prev => syncModesWithGlobalUsage(prev, json.quota));
      setSummaryHistoryVisible(true);
      setMessages(prev => {
        const switchTrace = json.switchTrace as Message | undefined;
        const welcome = json.welcomeMessage
          ? [{ id: -Date.now(), role: "assistant" as const, content: json.welcomeMessage, createdAt: new Date().toISOString(), modeId: json.modeId, modeName: json.modeName }]
          : [];
        if (newDialog) return [...(switchTrace ? [switchTrace] : []), ...welcome];
        const base = switchTrace ? replaceTrailingModeSwitchTrace(prev, switchTrace) : prev;
        return [...base, ...welcome];
      });
      if (newDialog) setBeforeId(null);
      setInput(preservedInput); setDialogCompleted(false);
    } catch (e) { setError(e instanceof Error ? e.message : "Ошибка выбора режима"); }
  }

  async function openBaseMode(preservedInput = "", newDialog = false) {
    setModeId(""); setModeName("Базовый ИИ"); setQuota(null); setError(null); setErrorDebug(null); setOrchestrationNotice(""); setLockedModeNotice("");
    try {
      const res = await fetch(`${apiBase}/api/chat/select-mode`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ modeId: 0, newDialog }) });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(json.error || `HTTP ${res.status}`);
      if (!json.ok) throw new Error(json.error || "Не удалось открыть базовый режим");
      setDialogId(json.dialogId); setDialogAccessRequired(false); setSummaryHistoryVisible(true);
      setModeName("Базовый ИИ"); setQuota(json.quota || null); if (json.quota) setModes(prev => syncModesWithGlobalUsage(prev, json.quota)); setMessages([]); setBeforeId(null); setInput(preservedInput); setDialogCompleted(false);
    } catch (e) { setError(e instanceof Error ? e.message : "Ошибка базового режима"); }
  }

  async function sendMessage(rawText?: string) {
    const preparedText = rawText ?? input;
    const text = cleanOutgoingMessage(preparedText);
    const hasText = text.length > 0;
    if (hasText && chatMessageMaxChars > 0 && [...text].length > chatMessageMaxChars) {
      setError(`Превышен лимит символов в сообщении: максимум ${chatMessageMaxChars}. Удалите часть текста и отправьте снова.`);
      return;
    }
    if (!dialogId || dialogCompleted || dialogAccessRequired || dailyLimitExhausted || (!hasText && attachments.length === 0)) return;
    setSending(true); setError(null); setErrorDebug(null);
    // #7 Optimistic UI: show the user's message IMMEDIATELY, do not wait for the AI response.
    // Create a temporary entry with a negative id (the real id comes in json.user).
    const attachmentIds = Array.from(new Set(attachments.map(item => item.id).filter(id => id > 0)));
    const outgoingText = text || (attachmentIds.length > 0 ? "Проанализируй приложенные файлы." : "");
    if (!outgoingText) { setSending(false); return; }
    const optimisticId = -Date.now();
    const optimisticUserMsg = { id: optimisticId, role: "user", content: outgoingText, createdAt: new Date().toISOString() };
    setMessages(prev => [...prev, optimisticUserMsg as any]);
    setInput(""); setSummaryHistoryVisible(false);
    scrollChatToBottom();
    try {
      const res = await fetch(`${apiBase}/api/chat/send`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ dialogId, text: outgoingText, responseMode, knowledgeModeIds: selectedModeIds, attachmentIds }) });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) {
        // Roll back the optimistic message on error.
        setMessages(prev => prev.filter(m => m.id !== optimisticId));
      if (res.status === 429) {
			  const updatedQuota = json.quota || quota;
			  setQuota(updatedQuota);
			  if (updatedQuota) setModes(prev => syncModesWithGlobalUsage(prev, updatedQuota));
			  setInput(outgoingText);
			  return;
			}
        throw new ApiError(json.error || `HTTP ${res.status}`, res.status, json);
      }
      if (!json.ok) {
        setMessages(prev => prev.filter(m => m.id !== optimisticId));
        throw new ApiError(json.error || "Не удалось отправить сообщение", res.status, json);
      }
      let switchTrace: Message | null = null;
      if (typeof json.modeId === "number") {
        const nextModeId = json.modeId;
        const nextModeName = json.modeName || modes.find(m => m.id === nextModeId)?.name || modeName;
        setModeId(nextModeId);
        setModeName(nextModeName);
        if (!selectedModeIds.includes(nextModeId)) setSelectedModeIds(prev => [nextModeId, ...prev.filter(id => id !== nextModeId)]);
        if (json.modeSwitched && nextModeName) {
          switchTrace = (json.switchTrace as Message | undefined) || {
            id: -Date.now() - 1,
            role: "system",
            content: modeSwitchTraceContent(modeName, nextModeName),
            createdAt: new Date().toISOString(),
          };
          setOrchestrationNotice(`для этой задачи лучше подойдёт режим «${nextModeName}» — переключаю`);
        }
      }
      setAttachments([]);
      // Replace the optimistic msg with the real one (with the real id from the DB) + add the assistant.
      setMessages(prev => prev.filter(m => m.id !== optimisticId).concat(switchTrace ? [json.user, switchTrace, json.assistant] : [json.user, json.assistant]));
      const updatedQuota = json.quota || quota; setQuota(updatedQuota);
      if (updatedQuota && typeof updatedQuota.remaining === "number") setModes(prev => syncModesWithGlobalUsage(prev, updatedQuota));
      scrollChatToBottom();
    } catch (e) {
      setMessages(prev => prev.filter(m => m.id !== optimisticId));
      setError(e instanceof Error ? e.message : "Ошибка отправки сообщения"); setErrorDebug(e instanceof ApiError ? e.debug : null);
    }
    finally { setSending(false); }
  }

  async function completeDialog() {
    if (!dialogId || !modeId || dialogAccessRequired || completing) return;
    setCompleting(true); setError(null); setErrorDebug(null);
    try {
      const res = await fetch(`${apiBase}/api/chat/complete`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ dialogId, responseMode }) });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) throw new ApiError(json.error || `HTTP ${res.status}`, res.status, json);
      if (!json.ok) throw new ApiError(json.error || "Не удалось завершить диалог", res.status, json);
      setSummaryHistoryVisible(true); setMessages(prev => [...prev, json.summary]);
      setQuota(json.quota || quota);
		if (json.quota) setModes(prev => syncModesWithGlobalUsage(prev, json.quota));
      scrollChatToBottom(); setDialogCompleted(true);
    } catch (e) { setError(e instanceof Error ? e.message : "Ошибка завершения диалога"); setErrorDebug(e instanceof ApiError ? e.debug : null); }
    finally { setCompleting(false); }
  }

  async function startNewDialogAfterSummary(preservedInput: string) {
    if (startingAfterSummary) return;
    setStartingAfterSummary(true);
    try { if (modeId) await openMode(Number(modeId), preservedInput, true); else await openBaseMode(preservedInput, true); }
    finally { setStartingAfterSummary(false); }
  }

  return { start, loadHistory, openMode, openBaseMode, sendMessage, completeDialog, startNewDialogAfterSummary };
}
