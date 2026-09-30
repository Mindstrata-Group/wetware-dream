"use client";

import Link from "next/link";
import { BrandLogo } from "@/components/BrandLogo";
import { T, btnReset, iconBtnStyle } from "../theme";
import { formatQuota } from "../utils";
import type { useChatPageController } from "../_hooks/useChatPageController";

export function ChatTopBar({ controller }: { controller: ReturnType<typeof useChatPageController> }) {
  const {
    loading, error, errorDebug, modes, quota, modeId, selectedModeIds, dialogId, modeName, input,
    sending, completing, dialogCompleted, dialogAccessRequired, responseMode, userRole, userEmail,
    historySearch, orchestrationNotice, lockedModeNotice, summaryHistoryVisible, canChooseResponseMode, emailInitial, showModes, showHistory,
    showProfile, showSearch, showJumpToBottom, chatSearchQ, showHistMenu, showProfileMenu, hoverChatId,
    editingHistoryId, editingHistoryTitle, idleDots, mobile, fold, desktop, compact, summaryMessages,
    chatScrollRef, textareaRef, topSentinelRef, historyMenuRef, profileMenuRef, scrollChatToBottom, sendMessage,
    completeDialog, handleInputChange, resetContext, restoreDialog, deleteHistoryItem, clearAllHistory, togglePin,
    renameHistory, saveHistoryRename, toggleModeSelection, selectAllModes, clearModeSelection, dailyLimitExhausted,
    canSend, handleTextareaKeyDown, activeHistoryItem, filteredHistory, visibleConversation, padH, headerH,
    chatSearchMatchCount, chatSearchActivePosition, goToNextChatSearchMatch, goToPreviousChatSearchMatch,
    bottomNavH, setInput, setResponseMode, setShowModes, setShowHistory, setShowProfile, setShowSearch,
    setChatSearchQ, setShowHistMenu, setShowProfileMenu, setHoverChatId, setEditingHistoryId,
    setEditingHistoryTitle, setIdleDots,
  } = controller;

  const { orchestrationLabel, currentModeLabel } = controller;
  return (
    <>
        {/* ── TOP BAR ── */}
        <header style={{ height: headerH, background: T.surface, borderBottom: `1px solid ${T.ink10}`, display: "flex", alignItems: "center", gap: 8, padding: `0 ${padH}px`, flexShrink: 0, position: "relative", zIndex: 10 }}>
          <BrandLogo priority />

          <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: desktop ? 12 : 6, flexShrink: 0 }}>
            <span style={{ fontSize: 12, padding: "4px 10px", borderRadius: 999, background: dailyLimitExhausted ? "#FBF0F0" : T.surfaceSoft, color: dailyLimitExhausted ? T.red : T.ink70, border: `1px solid ${dailyLimitExhausted ? "#EFB9B9" : T.ink20}`, whiteSpace: "nowrap" }}>
              {mobile
                ? `${quota?.remaining ?? Math.max((quota?.limit ?? 0) - (quota?.used ?? 0), 0)} / ${quota?.limit ?? "—"}`
                : (dailyLimitExhausted ? "Лимит на сегодня исчерпан" : formatQuota(quota))
              }
            </span>
            {desktop && (
              <button onClick={() => setShowModes(true)} style={{ ...btnReset, height: 32, padding: "0 12px", borderRadius: 999, border: `1px solid ${T.ink20}`, background: T.surfaceSoft, color: T.ink70, fontSize: 12, fontWeight: 600, whiteSpace: "nowrap" }}>
                {orchestrationLabel}
              </button>
            )}

            <Link href="/access?force=promo" style={{ ...btnReset, height: 34, padding: mobile ? "0 10px" : "0 14px", borderRadius: T.radiusSm, background: dailyLimitExhausted ? T.green : "transparent", color: dailyLimitExhausted ? "#fff" : T.greenDark, border: dailyLimitExhausted ? "none" : `1px solid ${T.ink10}`, fontSize: 13, fontWeight: 500 }}>
              {mobile ? "Продлить" : "Продлить доступ"}
            </Link>

            {desktop && canChooseResponseMode && (
              <label style={{ display: "flex", alignItems: "center", gap: 5, fontSize: 12, color: T.ink70, cursor: "pointer", userSelect: "none" }}>
                <input type="checkbox" checked={responseMode === "live"} onChange={e => setResponseMode(e.target.checked ? "live" : "test")} style={{ accentColor: T.green }} />
                {!fold && "Реальный AI"}
              </label>
            )}

          </div>
        </header>
        {mobile && (
          <div style={{ flexShrink: 0, display: "flex", justifyContent: "center", gap: 8, padding: "6px 14px 8px", background: T.surface, borderBottom: `1px solid ${T.ink10}` }}>
            <button onClick={() => setShowModes(true)} style={{ ...btnReset, height: 30, padding: "0 12px", borderRadius: 8, border: `1px solid ${T.ink20}`, background: T.surfaceSoft, color: T.ink70, fontSize: 12, gap: 6 }}>
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6"><path d="M3 3v10a1 1 0 001 1h9V4a1 1 0 00-1-1H3z"/><path d="M3 11h10"/></svg>
              {currentModeLabel || "Базовый ИИ"}
            </button>
            {canChooseResponseMode && (
              <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, color: T.ink70, cursor: "pointer", userSelect: "none", padding: "0 6px", borderRadius: 8, border: `1px solid ${T.ink20}`, background: T.surfaceSoft }}>
                <input type="checkbox" checked={responseMode === "live"} onChange={e => setResponseMode(e.target.checked ? "live" : "test")} style={{ accentColor: T.green }} />
                Реальный AI
              </label>
            )}
          </div>
        )}

        {/* ── DESKTOP: in-chat search bar ── */}
        {desktop && showSearch && (
          <div style={{ padding: "8px 24px", background: T.surface, borderBottom: `1px solid ${T.ink10}`, display: "flex", gap: 8, alignItems: "center", flexShrink: 0 }}>
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke={T.ink50} strokeWidth="1.6"><circle cx="7" cy="7" r="5"/><path d="M12 12l3 3"/></svg>
            <input
              autoFocus
              placeholder="Поиск по чату и истории"
              value={chatSearchQ}
              onChange={e => setChatSearchQ(e.target.value)}
              onInput={e => setChatSearchQ(e.currentTarget.value)}
              onKeyDown={e => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  if (e.shiftKey) goToPreviousChatSearchMatch(); else goToNextChatSearchMatch();
                }
              }}
              style={{ flex: 1, border: "none", outline: "none", background: "transparent", fontFamily: T.fontBody, fontSize: 14 }}
            />
            <span style={{ minWidth: 74, textAlign: "right", fontSize: 12, color: T.ink50 }}>
              {chatSearchQ ? (chatSearchMatchCount > 0 ? `${chatSearchActivePosition} / ${chatSearchMatchCount}` : "0 совпадений") : ""}
            </span>
            <button type="button" onClick={goToPreviousChatSearchMatch} disabled={chatSearchMatchCount === 0} title="Предыдущее совпадение" style={iconBtnStyle({ color: chatSearchMatchCount === 0 ? T.ink20 : T.ink70 })}>↑</button>
            <button type="button" onClick={goToNextChatSearchMatch} disabled={chatSearchMatchCount === 0} title="Следующее совпадение" style={iconBtnStyle({ color: chatSearchMatchCount === 0 ? T.ink20 : T.ink70 })}>↓</button>
            <button onClick={() => { setShowSearch(false); setChatSearchQ(""); }} style={iconBtnStyle()}>×</button>
          </div>
        )}
    </>
  );
}
