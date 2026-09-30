"use client";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
writeStoredSelectedModeIds, readHistory, writeHistory, buildHistoryTitle, buildHistorySnippet, buildHistorySearchText,
} from "../storage";
import {
	quotaExhausted, modeHasMessages, selectableModeIds, buildQuotaWarning, cleanOutgoingMessage,
	apiBase, collapseAdjacentModeSwitchTraces,
} from "../utils";
import { getBoolean, getNumber, getString } from "@/lib/siteContent";
import { useSiteContent } from "@/lib/useSiteContent";
import { createChatApiActions } from "./chatApiActions";
import { createChatHistoryActions } from "./chatHistoryActions";
import type { ChatAttachment, ChatHistoryItem, DailyQuota, Message, Mode } from "../types";
export function useChatPageController() {
	const siteContent = useSiteContent();
	const [loading, setLoading] = useState(true);
const [error, setError] = useState<string | null>(null);
const [errorDebug, setErrorDebug] = useState<unknown>(null);
const [modes, setModes] = useState<Mode[]>([]);
const [quota, setQuota] = useState<DailyQuota | null>(null);
const [chatMessageMaxChars, setChatMessageMaxChars] = useState(30 * 1024);
const [modeId, setModeId] = useState<number | "">("");
const [selectedModeIds, setSelectedModeIds] = useState<number[]>([]);
const [dialogId, setDialogId] = useState<number | null>(null);
const [modeName, setModeName] = useState("");
const [messages, setMessages] = useState<Message[]>([]);
const [input, setInput] = useState("");
const [attachments, setAttachments] = useState<ChatAttachment[]>([]);
const [attachmentError, setAttachmentError] = useState("");
const [uploadingAttachment, setUploadingAttachment] = useState(false);
const [dragActive, setDragActive] = useState(false);
const [sending, setSending] = useState(false);
const [completing, setCompleting] = useState(false);
const [dialogCompleted, setDialogCompleted] = useState(false);
const [dialogAccessRequired, setDialogAccessRequired] = useState(false);
const [startingAfterSummary, setStartingAfterSummary] = useState(false);
const [responseMode, setResponseMode] = useState("test");
const [userRole, setUserRole] = useState("");
const [userEmail, setUserEmail] = useState("");
const [maxLinked, setMaxLinked] = useState(false);
const [maxBotLink, setMaxBotLink] = useState("");
const maxLinkPollingRef = useRef<number | null>(null);
const [beforeId, setBeforeId] = useState<number | null>(null);
const [historySearch, setHistorySearch] = useState("");
const [historyItems, setHistoryItems] = useState<ChatHistoryItem[]>([]);
const [orchestrationNotice, setOrchestrationNotice] = useState("");
const [lockedModeNotice, setLockedModeNotice] = useState("");
const [summaryHistoryVisible, setSummaryHistoryVisible] = useState(true);
const [promoModeIds, setPromoModeIds] = useState<number[]>([]);
const [showModes, setShowModes] = useState(false);
const [showHistory, setShowHistory] = useState(false);
const [showProfile, setShowProfile] = useState(false);
const [showSearch, setShowSearch] = useState(false);
const [showJumpToBottom, setShowJumpToBottom] = useState(false);
const [chatSearchActiveIndex, setChatSearchActiveIndex] = useState(0);
const chatSearchQ = historySearch;
const setChatSearchQ = useCallback((value: string | ((current: string) => string)) => {
setHistorySearch(current => typeof value === "function" ? value(current) : value);
}, []);
const [showHistMenu, setShowHistMenu] = useState(false);
const [showProfileMenu, setShowProfileMenu] = useState(false);
const [hoverChatId, setHoverChatId] = useState<number | null>(null);
const [editingHistoryId, setEditingHistoryId] = useState<number | null>(null);
const [editingHistoryTitle, setEditingHistoryTitle] = useState("");
const [idleDots, setIdleDots] = useState(false);
const idleTimerRef = useRef<number | null>(null);
const [vp, setVp] = useState({ mobile: false, fold: false, desktop: true });
useEffect(() => {
const check = () => { const w = window.innerWidth; setVp({ fold: w < 390, mobile: w < 768, desktop: w >= 1024 }); };
check(); window.addEventListener("resize", check); return () => window.removeEventListener("resize", check);
}, []);
const{mobile,fold,desktop}=vp;
const compact=!desktop;
const conversationMessages = useMemo(() => messages.filter(m => m.role !== "summary"), [messages]);
const summaryMessages = useMemo(() => messages.filter(m => m.role === "summary"), [messages]);
const chatScrollRef = useRef<HTMLDivElement | null>(null);
const chatMessageRefs = useRef<Map<number, HTMLDivElement>>(new Map());
const textareaRef = useRef<HTMLTextAreaElement | null>(null);
const attachTextarea = useCallback((node: HTMLTextAreaElement | null) => {
textareaRef.current = node;
}, []);
const fileInputRef = useRef<HTMLInputElement | null>(null);
const topSentinelRef = useRef<HTMLDivElement | null>(null);
const modeSelectionLoadedRef = useRef<boolean>(false);
const historyMenuRef = useRef<HTMLDivElement | null>(null);
const profileMenuRef = useRef<HTMLDivElement | null>(null);
const loadingMoreRef = useRef<boolean>(false);
// IME composition (Android Gboard voice / Chinese/Japanese keyboards): keep it in a ref
// so we do not call setState and do not break the active composition buffer.
const isComposingRef = useRef<boolean>(false);
// Buffer of client textarea events for diagnostics.
// Flushed to /api/debug/client-log every 5 seconds.
const clientLogBufRef = useRef<object[]>([]);
const pushClientLog = useCallback((type: string, data: object = {}) => {
  clientLogBufRef.current.push({ type, ts: Date.now(), ...data });
}, []);
useEffect(() => {
  const flush = () => {
    const buf = clientLogBufRef.current;
    if (buf.length === 0) return;
    clientLogBufRef.current = [];
    void fetch(`${apiBase}/api/debug/client-log`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        events: buf,
        ua: typeof navigator !== "undefined" ? navigator.userAgent : "",
        path: typeof window !== "undefined" ? window.location.pathname : "",
      }),
    }).catch(() => { /* diagnostics: ignore network errors */ });
  };
  const id = window.setInterval(flush, 5000);
  return () => { window.clearInterval(id); flush(); };
}, []);
function scrollChatToBottom(behavior: ScrollBehavior = "smooth") {
const node = chatScrollRef.current; if (!node) return;
window.requestAnimationFrame(() => { node.scrollTo({ top: node.scrollHeight, behavior }); });
}
const setChatMessageRef = useCallback((messageId: number, node: HTMLDivElement | null) => {
if (!node) {
chatMessageRefs.current.delete(messageId);
return;
}
chatMessageRefs.current.set(messageId, node);
}, []);
useEffect(() => {
void start();
const loadLocalHistory=()=>setHistoryItems(readHistory());
if (typeof window !== "undefined" && "requestIdleCallback" in window) {
const idleId = window.requestIdleCallback(loadLocalHistory, { timeout: 500 });
return () => window.cancelIdleCallback(idleId);
}
const timer = setTimeout(loadLocalHistory, 0);
return () => clearTimeout(timer);
}, []);
useEffect(() => {
if (typeof window === "undefined") return;
const params = new URLSearchParams(window.location.search);
if (params.get("modes") === "1") setShowModes(true);
}, []);
const canChooseResponseMode=["admin","owner","tester"].includes(userRole);
const emailInitial=(userEmail.trim()[0]||userRole.trim()[0]||"U").toUpperCase();
useEffect(() => { if (userRole && !canChooseResponseMode && responseMode !== "live") setResponseMode("live"); }, [canChooseResponseMode, responseMode, userRole]);
useEffect(() => { if (typeof window !== "undefined" && canChooseResponseMode) localStorage.setItem("chat_response_mode", responseMode); }, [canChooseResponseMode, responseMode]);
useEffect(() => { if (!modeSelectionLoadedRef.current) return; writeStoredSelectedModeIds(selectedModeIds); }, [selectedModeIds]);
useEffect(() => { if (!orchestrationNotice) return; const t = window.setTimeout(() => setOrchestrationNotice(""), 6500); return () => window.clearTimeout(t); }, [orchestrationNotice]);
useEffect(() => {
if (!dialogId) return; if (!messages.some(m => m.id > 0)) return;
const nextTitle = buildHistoryTitle(modeName, messages, dialogId);
const nextSnippet = buildHistorySnippet(messages);
const nextSearchText = buildHistorySearchText(messages);
setHistoryItems(prev => {
const existing = prev.find(item => item.dialogId === dialogId);
const nextItem = { dialogId, modeId: existing?.modeId ?? (typeof modeId === "number" ? modeId : null), modeName: modeName || currentModeLabel || existing?.modeName || "", title: existing?.title || nextTitle, snippet: nextSnippet, searchText: nextSearchText, pinned: existing?.pinned || false, updatedAt: new Date().toISOString() };
const merged = existing ? prev.map(item => item.dialogId === dialogId ? { ...item, ...nextItem } : item) : [nextItem, ...prev];
writeHistory(merged); return merged;
});
}, [dialogId, messages, modeId, modeName]);
useEffect(() => {
const sentinel = topSentinelRef.current;
if (!sentinel || !beforeId) return;
const observer = new IntersectionObserver(
(entries) => {
if (entries[0].isIntersecting && dialogId && beforeId) {
void loadHistory(dialogId, true);
}
},
{ root: chatScrollRef.current, threshold: 0.1 }
);
observer.observe(sentinel);
return () => observer.disconnect();
}, [beforeId, dialogId]);
const messageLimit = Math.max(1, Math.floor(chatMessageMaxChars || 30 * 1024));
const cleanedInput = useMemo(() => cleanOutgoingMessage(input), [input]);
const inputLength = [...cleanedInput].length;
const messageLimitWarningThreshold = Math.max(1, Math.floor(messageLimit * 0.95));
const messageCharsRemaining = Math.max(messageLimit - inputLength, 0);
const hasMessageText = cleanedInput.length > 0;
const isMessageLimitReached = hasMessageText && inputLength >= messageLimit;
const isMessageLimitNear = hasMessageText && inputLength >= messageLimitWarningThreshold && !isMessageLimitReached;
const currentModeQuota=typeof modeId==="number"?(modes.find(m=>m.id===modeId)?.quota??quota):quota;
	const dailyLimitExhausted=quotaExhausted(currentModeQuota);
	const quotaWarning = buildQuotaWarning(currentModeQuota, {
	enabled: getBoolean(siteContent, "chat.quota.warning_enabled", true),
	thresholdRemaining: getNumber(siteContent, "chat.quota.warning_threshold_remaining", 3),
	thresholdPercent: getNumber(siteContent, "chat.quota.warning_threshold_percent", 20),
	title: getString(siteContent, "chat.quota.warning_title", "Сегодня осталось мало сообщений"),
	body: getString(siteContent, "chat.quota.warning_body", "Если вопрос важный, отправьте его одним сообщением. Осталось {{remaining}} из {{limit}}."),
	});
const apiActions = createChatApiActions({
	modes, quota, modeId, modeName, input, chatMessageMaxChars, attachments, dialogId, selectedModeIds, beforeId, responseMode,
dialogCompleted, dialogAccessRequired, dailyLimitExhausted, sending, completing, startingAfterSummary,
modeSelectionLoadedRef, loadingMoreRef, setLoading, setError, setErrorDebug, setUserRole, setUserEmail, setMaxLinked, setMaxBotLink,
	setResponseMode, setModes, setSelectedModeIds, setPromoModeIds, setQuota, setChatMessageMaxChars, setModeId, setModeName,
setDialogId, setMessages, setBeforeId, setSummaryHistoryVisible, setDialogAccessRequired,
setDialogCompleted, setInput, setAttachments, setOrchestrationNotice, setLockedModeNotice, setSending, setCompleting,
setStartingAfterSummary, scrollChatToBottom,
});
const { start, loadHistory, openMode, openBaseMode, sendMessage, completeDialog, startNewDialogAfterSummary } = apiActions;
const historyActions = createChatHistoryActions({
modes, modeId, input, dialogId, dialogCompleted, dialogAccessRequired, editingHistoryId, editingHistoryTitle,
historyItems, selectedModeIds, startingAfterSummary, openMode, openBaseMode, loadHistory, startNewDialogAfterSummary,
setInput, setSummaryHistoryVisible, setHistoryItems, setDialogId, setModeId, setModeName, setBeforeId,
setDialogCompleted, setDialogAccessRequired, setEditingHistoryId, setEditingHistoryTitle,
setLockedModeNotice, setSelectedModeIds, setOrchestrationNotice,
setError, setErrorDebug, setMessages, setHistorySearch,
});
const {
handleInputChange, resetContext, restoreDialog, deleteHistoryItem, clearAllHistory, togglePin, renameHistory,
activateModeManually, saveHistoryRename, toggleModeSelection, selectAllModes, clearModeSelection,
} = historyActions;
	  const canSend = useMemo(() => !!dialogId && !dialogCompleted && !dialogAccessRequired && !dailyLimitExhausted && (hasMessageText || attachments.length > 0) && !isMessageLimitReached && !sending && !uploadingAttachment && !startingAfterSummary, [dialogId, dialogCompleted, dialogAccessRequired, dailyLimitExhausted, hasMessageText, attachments.length, isMessageLimitReached, sending, uploadingAttachment, startingAfterSummary]);
	  function handleTextareaKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
	    if ((e.key !== "Enter" && e.key !== "NumpadEnter") || e.shiftKey) return;
	    // On a mobile keyboard nativeEvent.isComposing may be true during IME; do not send
	    if ((e.nativeEvent as KeyboardEvent).isComposing || isComposingRef.current) return;
	    const nextText = e.currentTarget.value;
	    const preparedText = cleanOutgoingMessage(nextText);
	    if ((!preparedText && attachments.length === 0) || (preparedText.length > 0 && [...preparedText].length >= messageLimit)) return;
	    if (!dialogId || dialogCompleted || dialogAccessRequired || dailyLimitExhausted || sending || uploadingAttachment || startingAfterSummary) return;
	    e.preventDefault();
	    void sendMessage(nextText);
	  }
function autoResizeTextarea(el: HTMLTextAreaElement | null) {
if (!el) return;
el.style.height = "auto";
el.style.height = `${Math.min(el.scrollHeight, 130)}px`;
}
const selectableIds=selectableModeIds(modes);
const isAllModesSelected=selectableIds.length>0&&selectableIds.every(id=>selectedModeIds.includes(id));
const isPromoModesSelected=promoModeIds.length>0&&promoModeIds.every(id=>selectedModeIds.includes(id));
const orchestrationLabel=isAllModesSelected?"Максимум опыта":isPromoModesSelected?"Максимум тренировки":"Выборочная работа";
const currentModeLabel = modeName || (modeId ? modes.find(m => m.id === modeId)?.name || "" : "");
const thinkingLabel = orchestrationNotice || `Помощник Стратум думает в режиме «${currentModeLabel || "Базовый ИИ"}»`;
const activeHistoryItem = historyItems.find(h => h.dialogId === dialogId);
const allowedAttachmentExtensions = useMemo(() => new Set(["txt", "md", "doc", "docx"]), []);
async function uploadAttachment(file: File) {
const ext = file.name.split(".").pop()?.toLowerCase() || "";
if (!allowedAttachmentExtensions.has(ext)) {
setAttachmentError("Поддерживаются только txt, md, doc и docx.");
return;
}
setAttachmentError("");
setUploadingAttachment(true);
try {
const body = new FormData();
body.append("file", file);
const res = await fetch(`${apiBase}/api/chat/attachments`, { method: "POST", credentials: "include", body });
const json = await res.json().catch(() => ({}));
if (!res.ok || json.ok === false || !json.attachment) throw new Error(json.error || `HTTP ${res.status}`);
const next = json.attachment as ChatAttachment;
setAttachments(prev => prev.some(item => item.id === next.id) ? prev : [...prev, next]);
} catch (e) {
setAttachmentError(e instanceof Error ? e.message : "Не удалось прикрепить файл");
} finally {
setUploadingAttachment(false);
}
}
async function uploadAttachmentList(files: FileList | File[]) {
for (const file of Array.from(files)) {
await uploadAttachment(file);
}
}
function handleAttachmentInputChange(e: React.ChangeEvent<HTMLInputElement>) {
const files = e.target.files;
if (files?.length) void uploadAttachmentList(files);
e.target.value = "";
}
function handleDrop(e: React.DragEvent<HTMLElement>) {
e.preventDefault();
e.stopPropagation();
setDragActive(false);
const files = e.dataTransfer.files;
if (files.length) void uploadAttachmentList(files);
}
function handleDragOver(e: React.DragEvent<HTMLElement>) {
e.preventDefault();
e.stopPropagation();
if (!dragActive) setDragActive(true);
}
function handleDragLeave(e: React.DragEvent<HTMLElement>) {
e.preventDefault();
e.stopPropagation();
const nextTarget = e.relatedTarget;
if (!nextTarget || !(nextTarget instanceof Node) || !e.currentTarget.contains(nextTarget)) setDragActive(false);
}
function removeAttachment(id: number) {
setAttachments(prev => prev.filter(item => item.id !== id));
}
useEffect(() => {
if (typeof modeId !== "number" || !dailyLimitExhausted || sending || completing) return;
const fallbackMode = modes.find(m => m.id !== modeId && modeHasMessages(m));
if (!fallbackMode) return;
void openMode(fallbackMode.id, input, true);
setOrchestrationNotice(`Лимит режима «${modeName || "—"}» исчерпан — переключаю на «${fallbackMode.name}».`);
}, [modeId, dailyLimitExhausted, modes, sending, completing]);
useEffect(() => {
if (loadingMoreRef.current) { loadingMoreRef.current = false; return; }
scrollChatToBottom();
}, [messages, dialogId, sending]);
useEffect(() => { autoResizeTextarea(textareaRef.current); }, [input]);
useEffect(() => {
const armIdle = () => {
if (idleTimerRef.current) window.clearTimeout(idleTimerRef.current);
setIdleDots(false);
idleTimerRef.current = window.setTimeout(() => setIdleDots(true), 10 * 60 * 1000);
};
armIdle();
const onActivity = () => armIdle();
window.addEventListener("pointerdown", onActivity, { passive: true });
window.addEventListener("keydown", onActivity);
return () => {
window.removeEventListener("pointerdown", onActivity);
window.removeEventListener("keydown", onActivity);
if (idleTimerRef.current) window.clearTimeout(idleTimerRef.current);
};
}, []);
useEffect(() => {
const node = chatScrollRef.current;
if (!node) return;
const onScroll = () => {
const isAwayFromBottom = node.scrollHeight - node.scrollTop - node.clientHeight > 180;
setShowJumpToBottom(isAwayFromBottom);
};
onScroll();
node.addEventListener("scroll", onScroll);
return () => node.removeEventListener("scroll", onScroll);
}, [dialogId, messages.length]);
useEffect(() => {
const onPointerDown = (event: PointerEvent) => {
const target = event.target as Node | null;
if (showHistMenu && !historyMenuRef.current?.contains(target)) setShowHistMenu(false);
if (showProfileMenu && !profileMenuRef.current?.contains(target)) setShowProfileMenu(false);
};
const onKeyDown = () => {
if (showHistMenu) setShowHistMenu(false);
if (showProfileMenu) setShowProfileMenu(false);
};
window.addEventListener("pointerdown", onPointerDown);
window.addEventListener("keydown", onKeyDown);
return () => {
window.removeEventListener("pointerdown", onPointerDown);
window.removeEventListener("keydown", onKeyDown);
};
}, [showHistMenu, showProfileMenu]);
const filteredHistory = [...historyItems].sort((a, b) => { if (a.pinned !== b.pinned) return a.pinned ? -1 : 1; return new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(); }).filter(item => { const needle = historySearch.trim().toLowerCase(); if (!needle) return true; return `${item.title} ${item.modeName} ${item.snippet} ${item.searchText}`.toLowerCase().includes(needle); });
const normalizedChatSearchQ = chatSearchQ.trim().toLowerCase();
const chatSearchMatchIds = useMemo(() => {
if (!normalizedChatSearchQ) return [];
return [...conversationMessages]
.reverse()
.filter(m => (m.content || "").toLowerCase().includes(normalizedChatSearchQ))
.map(m => m.id);
}, [conversationMessages, normalizedChatSearchQ]);
const chatSearchMatchCount = chatSearchMatchIds.length;
const chatSearchActiveMessageId = chatSearchMatchCount > 0
? chatSearchMatchIds[Math.min(chatSearchActiveIndex, chatSearchMatchCount - 1)]
: null;
const chatSearchActivePosition = chatSearchActiveMessageId === null ? 0 : Math.min(chatSearchActiveIndex + 1, chatSearchMatchCount);
useEffect(() => {
setChatSearchActiveIndex(0);
}, [normalizedChatSearchQ, chatSearchMatchIds.join(",")]);
useEffect(() => {
if (!normalizedChatSearchQ || chatSearchActiveMessageId === null) return;
const node = chatMessageRefs.current.get(chatSearchActiveMessageId);
if (!node || typeof node.scrollIntoView !== "function") return;
window.requestAnimationFrame(() => node.scrollIntoView({ behavior: "smooth", block: "center" }));
}, [chatSearchActiveMessageId, normalizedChatSearchQ]);
const goToNextChatSearchMatch = useCallback(() => {
setChatSearchActiveIndex(current => chatSearchMatchCount > 0 ? (current + 1) % chatSearchMatchCount : 0);
}, [chatSearchMatchCount]);
const goToPreviousChatSearchMatch = useCallback(() => {
setChatSearchActiveIndex(current => chatSearchMatchCount > 0 ? (current - 1 + chatSearchMatchCount) % chatSearchMatchCount : 0);
}, [chatSearchMatchCount]);
const visibleConversation = useMemo(() => collapseAdjacentModeSwitchTraces(conversationMessages), [conversationMessages]);
const padH=fold?12:mobile?14:20;
const headerH=mobile?52:64;
const bottomNavH=56;
const startMaxLinkPolling = useCallback(() => {
  if (maxLinkPollingRef.current) window.clearTimeout(maxLinkPollingRef.current);
  let attempts = 0;
  const poll = () => {
    attempts++;
    fetch(`${apiBase}/api/notifications/max/start-link`, { credentials: "include" })
      .then(r => r.ok ? r.json() : null)
      .then((data: { maxLinked?: boolean } | null) => {
        if (data?.maxLinked) { setMaxLinked(true); return; }
        if (attempts < 10) maxLinkPollingRef.current = window.setTimeout(poll, 3000);
      })
      .catch(() => { if (attempts < 10) maxLinkPollingRef.current = window.setTimeout(poll, 3000); });
  };
  maxLinkPollingRef.current = window.setTimeout(poll, 3000);
}, [apiBase]);
useEffect(() => () => { if (maxLinkPollingRef.current) window.clearTimeout(maxLinkPollingRef.current); }, []);
return {
	loading, error, errorDebug, modes, quota, modeId, selectedModeIds, dialogId, modeName, messages, input,
	chatMessageMaxChars, messageLimit, messageCharsRemaining: messageCharsRemaining, isMessageLimitNear, isMessageLimitReached, messageLength: inputLength, attachments, attachmentError, uploadingAttachment, dragActive, fileInputRef,
	sending, completing, dialogCompleted, dialogAccessRequired, startingAfterSummary, responseMode, userRole,
userEmail, maxLinked, maxBotLink, beforeId, historySearch, historyItems, orchestrationNotice, lockedModeNotice, summaryHistoryVisible,
promoModeIds, showModes, showHistory, showProfile, showSearch, showJumpToBottom, chatSearchQ, showHistMenu,
showProfileMenu, hoverChatId, editingHistoryId, editingHistoryTitle, idleDots, idleTimerRef, vp, mobile, fold,
desktop, compact, conversationMessages, summaryMessages, chatScrollRef, textareaRef, attachTextarea, topSentinelRef,
modeSelectionLoadedRef, historyMenuRef, profileMenuRef, loadingMoreRef, isComposingRef, pushClientLog, scrollChatToBottom, setChatMessageRef, start,
loadHistory, openBaseMode, openMode, sendMessage, completeDialog, startNewDialogAfterSummary, handleInputChange,
resetContext, restoreDialog, deleteHistoryItem, clearAllHistory, togglePin, renameHistory, activateModeManually,
	saveHistoryRename, toggleModeSelection, selectAllModes, clearModeSelection, currentModeQuota, dailyLimitExhausted, quotaWarning,
canSend, handleTextareaKeyDown, handleAttachmentInputChange, handleDrop, handleDragOver, handleDragLeave, removeAttachment,
autoResizeTextarea, selectableIds, isAllModesSelected, isPromoModesSelected,
orchestrationLabel, currentModeLabel, thinkingLabel, activeHistoryItem, filteredHistory, visibleConversation, canChooseResponseMode, emailInitial,
chatSearchMatchIds, chatSearchMatchCount, chatSearchActiveMessageId, chatSearchActivePosition, goToNextChatSearchMatch, goToPreviousChatSearchMatch,
padH, headerH, bottomNavH, startMaxLinkPolling, setLoading, setError, setErrorDebug, setModes, setQuota, setModeId, setSelectedModeIds,
setDialogId, setModeName, setMessages, setInput, setAttachments, setAttachmentError, setUploadingAttachment, setDragActive,
setSending, setCompleting, setDialogCompleted, setDialogAccessRequired,
setStartingAfterSummary, setResponseMode, setUserRole, setUserEmail, setBeforeId, setHistorySearch, setHistoryItems,
setOrchestrationNotice, setLockedModeNotice, setSummaryHistoryVisible, setPromoModeIds, setShowModes, setShowHistory,
setShowProfile, setShowSearch, setShowJumpToBottom, setChatSearchQ, setShowHistMenu, setShowProfileMenu,
setHoverChatId, setEditingHistoryId, setEditingHistoryTitle, setIdleDots, setVp,
};
}
