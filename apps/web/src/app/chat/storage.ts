// SOLID-split of chat/page.tsx 2026-05-31: localStorage I/O for history and mode selection.

import type {
  ChatHistoryItem,
  Message,
  Mode,
  StoredModeSelection,
} from "./types";

export const HISTORY_KEY = "ms_chat_history_v5";
export const SELECTED_MODE_IDS_KEY = "ms_chat_selected_mode_ids";
export const LAST_SELECTED_MODE_IDS_KEY = "ms_chat_last_selected_mode_ids_v1";

export function uniqueAvailableModeIds(ids: number[], availableModes: Mode[]) {
  const available = new Set(availableModes.map(m => m.id));
  return [...new Set(ids.filter(id => Number.isFinite(id) && available.has(id)))];
}

export function readPendingSelectedModeIds(availableModes: Mode[]) {
  if (typeof window === "undefined") return null;
  const fromQuery = new URLSearchParams(window.location.search).get("modes")?.split(",").map(v => Number(v.trim())).filter(v => Number.isFinite(v));
  if (fromQuery?.length) {
    window.history.replaceState(null, "", window.location.pathname);
    localStorage.removeItem(SELECTED_MODE_IDS_KEY);
    return uniqueAvailableModeIds(fromQuery, availableModes);
  }
  try {
    const raw = localStorage.getItem(SELECTED_MODE_IDS_KEY);
    if (!raw) return null;
    localStorage.removeItem(SELECTED_MODE_IDS_KEY);
    const parsed = JSON.parse(raw);
    const ids = Array.isArray(parsed) ? parsed.filter((id): id is number => typeof id === "number") : [];
    return uniqueAvailableModeIds(ids, availableModes);
  } catch {
    return null;
  }
}

export function readStoredSelectedModeIds(availableModes: Mode[]): StoredModeSelection {
  if (typeof window === "undefined") return { ids: [], exists: false };
  try {
    const raw = localStorage.getItem(LAST_SELECTED_MODE_IDS_KEY);
    if (!raw) return { ids: [], exists: false };
    const parsed = JSON.parse(raw);
    const ids = Array.isArray(parsed) ? parsed.filter((id): id is number => typeof id === "number") : [];
    const validIds = uniqueAvailableModeIds(ids, availableModes);
    return { ids: validIds, exists: ids.length === 0 || validIds.length > 0 };
  } catch {
    return { ids: [], exists: false };
  }
}

export function writeStoredSelectedModeIds(ids: number[]) {
  if (typeof window === "undefined") return;
  localStorage.setItem(LAST_SELECTED_MODE_IDS_KEY, JSON.stringify(ids));
}

export function readHistory(): ChatHistoryItem[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function writeHistory(items: ChatHistoryItem[]) {
  if (typeof window === "undefined") return;
  localStorage.setItem(HISTORY_KEY, JSON.stringify(items));
}

export function buildHistoryTitle(modeName: string, messages: Message[], dialogId: number) {
  const firstUser = messages.find(m => m.role === "user")?.content?.trim();
  if (firstUser) return firstUser.slice(0, 56);
  if (modeName.trim()) return modeName.trim();
  return `Диалог ${dialogId}`;
}

export function buildHistorySnippet(messages: Message[]) {
  const last = [...messages].reverse().find(m => m.role === "assistant" || m.role === "summary" || m.role === "user");
  return last?.content?.replace(/\s+/g, " ").trim().slice(0, 150) || "";
}

export function buildHistorySearchText(messages: Message[]) {
  return messages.map(m => `${m.role} ${m.content}`).join(" ").replace(/\s+/g, " ").trim().toLowerCase();
}
