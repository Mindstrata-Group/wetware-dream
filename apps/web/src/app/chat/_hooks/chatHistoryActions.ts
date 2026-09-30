"use client";

import type { ChatHistoryItem } from "../types";
import type { ChatHistoryActionsContext } from "./chatActionTypes";
import { apiBase, dailyUnlockLabel, modeHasMessages, selectableModeIds } from "../utils";
import { writeHistory } from "../storage";

export function createChatHistoryActions(ctx: ChatHistoryActionsContext) {
  const {
    modes, modeId, input, dialogId, dialogCompleted, dialogAccessRequired, editingHistoryId, editingHistoryTitle,
    historyItems, selectedModeIds, startingAfterSummary, openMode, openBaseMode, loadHistory, startNewDialogAfterSummary,
    setInput, setSummaryHistoryVisible, setHistoryItems, setDialogId, setModeId, setModeName, setBeforeId,
    setDialogCompleted, setDialogAccessRequired, setEditingHistoryId, setEditingHistoryTitle,
    setLockedModeNotice, setSelectedModeIds, setOrchestrationNotice,
    setError, setErrorDebug, setMessages, setHistorySearch,
  } = ctx;

  function handleInputChange(nextValue: string) {
    setInput(nextValue);
    if (nextValue.trim().length > 0) setSummaryHistoryVisible(false);
    if (dialogAccessRequired) return;
    if (dialogCompleted && nextValue.trim().length > 0) void startNewDialogAfterSummary(nextValue);
  }

  async function resetContext() {
    const currentModeAvailable = typeof modeId === "number" && modes.some(m => m.id === modeId);
    if (dialogAccessRequired || (modeId && !currentModeAvailable)) { await openBaseMode(input, true); return; }
    if (modeId) { await openMode(Number(modeId), "", true); return; }
    await openBaseMode("", true);
  }

  async function restoreDialog(item: ChatHistoryItem) {
    setDialogId(item.dialogId); setModeId(item.modeId ?? ""); setModeName(item.modeName); setBeforeId(null); setError(null); setErrorDebug(null); setInput(""); setDialogCompleted(false); setDialogAccessRequired(false);
    if (item.modeId) setSelectedModeIds(prev => prev.includes(item.modeId as number) ? prev : [item.modeId as number, ...prev]);
    const loaded = await loadHistory(item.dialogId);
    if (!loaded) { setHistoryItems(prev => { const next = prev.filter(hi => hi.dialogId !== item.dialogId); writeHistory(next); return next; }); await openBaseMode(); }
  }

  async function deleteHistoryItem(dialogIdToDelete: number) {
    if (!window.confirm("Удалить этот чат из истории?")) return;
    try { await fetch(`${apiBase}/api/chat/history?dialogId=${dialogIdToDelete}`, { method: "DELETE", credentials: "include" }); } catch { }
    setHistoryItems(prev => { const next = prev.filter(item => item.dialogId !== dialogIdToDelete); writeHistory(next); return next; });
    if (dialogId === dialogIdToDelete) await openBaseMode();
  }

  async function clearAllHistory() {
    if (!window.confirm("Очистить всю историю чатов на этом устройстве и сервере?")) return;
    try { await fetch(`${apiBase}/api/chat/history?all=true`, { method: "DELETE", credentials: "include" }); } catch { }
    writeHistory([]); setHistoryItems([]); setMessages([]); setSummaryHistoryVisible(true);
    setDialogId(null); setModeId(""); setModeName(""); setBeforeId(null); setDialogCompleted(false); setDialogAccessRequired(false); setInput("");
    // #11: newDialog=true forces creation of a new empty dialog on the server.
    // Without the flag openBaseMode could reopen the current dialog with old messages.
    await openBaseMode("", true);
  }

  function togglePin(dialogIdToToggle: number) {
    setHistoryItems(prev => { const next = prev.map(item => item.dialogId === dialogIdToToggle ? { ...item, pinned: !item.pinned } : item); writeHistory(next); return next; });
  }

  function renameHistory(dialogIdToRename: number) {
    const current = historyItems.find(item => item.dialogId === dialogIdToRename);
    if (!current) return;
    setEditingHistoryId(dialogIdToRename);
    setEditingHistoryTitle(current.title || "");
  }

  async function activateModeManually(nextModeId: number) {
    await openMode(nextModeId, input);
    setOrchestrationNotice(`Режим переключен вручную на «${modes.find(m => m.id === nextModeId)?.name || "Базовый ИИ"}»`);
  }

  function saveHistoryRename() {
    if (!editingHistoryId) return;
    const nextTitle = editingHistoryTitle.trim();
    if (!nextTitle) { setEditingHistoryId(null); return; }
    setHistoryItems(prev => { const next = prev.map(item => item.dialogId === editingHistoryId ? { ...item, title: nextTitle } : item); writeHistory(next); return next; });
    setEditingHistoryId(null);
  }

  async function toggleModeSelection(nextModeId: number) {
    const targetMode = modes.find(m => m.id === nextModeId);
    if (targetMode && !modeHasMessages(targetMode)) { setLockedModeNotice(`До разблокировки: ${dailyUnlockLabel()}. Другие режимы можно использовать.`); return; }
    setLockedModeNotice("");
    const isSelected = selectedModeIds.includes(nextModeId);
    const isCurrent = modeId === nextModeId;

    if (!isSelected) {
      const nextSelected = [nextModeId, ...selectedModeIds];
      setSelectedModeIds(nextSelected);
      void activateModeManually(nextModeId);
      return;
    }

    if (!isCurrent) {
      setSelectedModeIds(prev => [nextModeId, ...prev.filter(id => id !== nextModeId)]);
      void activateModeManually(nextModeId);
      return;
    }

    const availableAlternatives = modes.filter(m => m.id !== nextModeId && modeHasMessages(m));
    const randomAlternative = availableAlternatives.length > 0
      ? availableAlternatives[Math.floor(Math.random() * availableAlternatives.length)]
      : null;
    const switchedModeId = randomAlternative?.id ?? nextModeId;
    setSelectedModeIds(prev => [switchedModeId, ...prev.filter(id => id !== nextModeId && id !== switchedModeId)]);
    await openMode(switchedModeId, "");
    const nextMode = modes.find(m => m.id === switchedModeId);
    setOrchestrationNotice(`Режим переключен вручную на «${nextMode?.name || "Базовый ИИ"}»`);
  }

  function selectAllModes() { setSelectedModeIds(selectableModeIds(modes)); }
  function clearModeSelection() { setSelectedModeIds([]); setModeId(""); setModeName("Базовый ИИ"); void openBaseMode(input, true); }

  return {
    handleInputChange, resetContext, restoreDialog, deleteHistoryItem, clearAllHistory, togglePin, renameHistory,
    activateModeManually, saveHistoryRename, toggleModeSelection, selectAllModes, clearModeSelection,
  };
}
