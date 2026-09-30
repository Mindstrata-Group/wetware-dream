"use client";

import { T, btnReset } from "../theme";
import type { useChatPageController } from "../_hooks/useChatPageController";

export function ChatMobileBottomNav({ controller }: { controller: ReturnType<typeof useChatPageController> }) {
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
    bottomNavH, setInput, setResponseMode, setShowModes, setShowHistory, setShowProfile, setShowSearch,
    setChatSearchQ, setShowHistMenu, setShowProfileMenu, setHoverChatId, setEditingHistoryId,
    setEditingHistoryTitle, setIdleDots,
  } = controller;

  return (
    <>
        {/* ── MOBILE BOTTOM NAV ── */}
        {compact && (
          <nav style={{ height: bottomNavH, background: T.surface, borderTop: `1px solid ${T.ink10}`, display: "flex", alignItems: "center", flexShrink: 0, zIndex: 10 }}>
            {[
              { id: "history", label: "История", icon: "◷", active: false, onClick: () => setShowHistory(true), isFab: false },
              { id: "new",     label: "Новый",   icon: "+",  active: false, onClick: () => void resetContext(), isFab: true },
              { id: "profile", label: "Профиль", icon: emailInitial, active: false, onClick: () => setShowProfile(true), isFab: false },
            ].map(n => (
              <button key={n.id} onClick={n.onClick} style={{ ...btnReset, flex: 1, flexDirection: "column", gap: 2, height: "100%", color: n.active ? T.greenDark : "#6E8E83", fontFamily: T.fontBody }}>
                {n.isFab ? (
                  <div style={{ width: 38, height: 38, borderRadius: "50%", background: T.green, color: "#fff", display: "flex", alignItems: "center", justifyContent: "center", marginTop: -6, fontSize: 20 }}>+</div>
                ) : n.id === "profile" ? (
                  <div style={{ width: 22, height: 22, borderRadius: "50%", background: T.greenLight, color: T.greenDark, display: "flex", alignItems: "center", justifyContent: "center", fontSize: 11, fontWeight: 700, lineHeight: 1 }}>{n.icon}</div>
                ) : (
                  <span style={{ fontSize: 16, lineHeight: 1 }}>{n.icon}</span>
                )}
                <span style={{ fontSize: 10, fontWeight: 500 }}>{n.label}</span>
              </button>
            ))}
          </nav>
        )}
    </>
  );
}
