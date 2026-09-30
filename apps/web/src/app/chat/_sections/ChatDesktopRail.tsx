"use client";

import { T, btnReset, iconBtnStyle, popoverStyle } from "../theme";
import { formatDate, formatQuota } from "../utils";
import { PopItem } from "../_components/PopItem";
import type { useChatPageController } from "../_hooks/useChatPageController";

export function ChatDesktopRail({ controller }: { controller: ReturnType<typeof useChatPageController> }) {
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
          {/* LEFT RAIL (desktop) */}
          {desktop && (
            <aside style={{ width: 240, flexShrink: 0, background: T.surface, borderRight: `1px solid ${T.ink10}`, display: "flex", flexDirection: "column" }}>
              {/* new chat */}
              <div style={{ padding: "12px 14px", borderBottom: `1px solid ${T.ink10}` }}>
                <div style={{ paddingBottom: 12 }}>
                  <button
                    onClick={() => void resetContext()}
                    style={{ ...btnReset, width: "100%", height: 40, borderRadius: 10, background: T.green, color: "#fff", fontWeight: 600, fontSize: 14, gap: 8, justifyContent: "center" }}
                  >
                    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round"><path d="M8 1v14M1 8h14"/></svg>
                    Новый чат
                  </button>
                </div>
              </div>

              {/* history: heading */}
              <div ref={historyMenuRef} style={{ padding: "8px 14px 4px", display: "flex", alignItems: "center", gap: 6, position: "relative" }}>
                <div style={{ fontSize: 11, color: T.ink50, textTransform: "uppercase", letterSpacing: "0.6px", fontWeight: 500, flex: 1 }}>История</div>
                <button onClick={() => setShowHistMenu(v => !v)} style={iconBtnStyle()} title="Меню истории">
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke={T.ink50} strokeWidth="1.6"><circle cx="3" cy="8" r="1" /><circle cx="8" cy="8" r="1" /><circle cx="13" cy="8" r="1" /></svg>
                </button>
                {showHistMenu && (
                  <div style={popoverStyle({ top: 28, right: 8, w: 200 })}>
                    <PopItem danger onClick={() => { setShowHistMenu(false); void clearAllHistory(); }}>Очистить историю</PopItem>
                  </div>
                )}
              </div>

              {/* chat list */}
              <div className="ms-chat-rail" style={{ flex: 1, overflowY: "auto", padding: "4px 8px 12px" }}>
                {filteredHistory.length === 0 ? (
                  <div style={{ padding: 16, color: T.ink50, fontSize: 13, textAlign: "center" }}>Пока нет сохранённых диалогов.</div>
                ) : filteredHistory.map(item => {
                  const active = item.dialogId === dialogId;
                  const hover = hoverChatId === item.dialogId;
                  return (
                    <div
                      key={item.dialogId}
                      onMouseEnter={() => setHoverChatId(item.dialogId)}
                      onMouseLeave={() => setHoverChatId(null)}
                      onClick={() => void restoreDialog(item)}
                      style={{ padding: "8px 10px", borderRadius: 8, cursor: "pointer", position: "relative", background: active ? T.greenLight : (hover ? T.surfaceSoft : "transparent"), color: active ? T.greenDark : T.ink, marginBottom: 2 }}
                    >
                      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                        {item.pinned && <span style={{ color: T.greenDark, fontSize: 12 }}>📌</span>}
                        {editingHistoryId === item.dialogId ? (
                          <input
                            autoFocus
                            value={editingHistoryTitle}
                            onChange={(e) => setEditingHistoryTitle(e.target.value)}
                            onBlur={saveHistoryRename}
                            onKeyDown={(e) => { if (e.key === "Enter") saveHistoryRename(); if (e.key === "Escape") setEditingHistoryId(null); }}
                            style={{ flex: 1, minWidth: 0, border: `1px solid ${T.ink20}`, borderRadius: 6, padding: "2px 6px", fontSize: 13, fontFamily: T.fontBody }}
                          />
                        ) : <span style={{ fontSize: 13, fontWeight: 500, flex: 1, minWidth: 0, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{item.title}</span>}
                        {hover && (
                          <button onClick={(e) => { e.stopPropagation(); void deleteHistoryItem(item.dialogId); }} style={iconBtnStyle({ color: T.red })} title="Удалить">×</button>
                        )}
                      </div>
                      <div style={{ fontSize: 11, color: active ? T.greenDark : T.ink50, marginTop: 2, display: "flex", justifyContent: "space-between" }}>
                        <span>{item.modeName || "—"}</span>
                        <span>{formatDate(item.updatedAt)}</span>
                      </div>
                      {hover && (
                        <div onClick={e => e.stopPropagation()} style={{ display: "flex", gap: 8, marginTop: 4 }}>
                          <button onClick={() => togglePin(item.dialogId)} style={{ ...btnReset, fontSize: 10, color: T.ink50, padding: "2px 6px" }}>{item.pinned ? "Открепить" : "Закрепить"}</button>
                          <button onClick={() => renameHistory(item.dialogId)} style={{ ...btnReset, fontSize: 10, color: T.ink50, padding: "2px 6px" }}>Переименовать</button>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>

              {/* profile */}
              <div ref={profileMenuRef} style={{ position: "relative", borderTop: `1px solid ${T.ink10}`, padding: 10 }}>
                <button
                  onClick={() => setShowProfileMenu(v => !v)}
                  style={{ width: "100%", display: "flex", gap: 10, alignItems: "center", padding: "6px 8px", borderRadius: 10, border: "none", background: showProfileMenu ? T.surfaceSoft : "transparent", cursor: "pointer", fontFamily: T.fontBody, textAlign: "left" }}
                >
                  <div style={{ width: 32, height: 32, borderRadius: "50%", background: T.greenLight, color: T.greenDark, display: "flex", alignItems: "center", justifyContent: "center", fontWeight: 600, fontSize: 13, flexShrink: 0 }}>
                    {(userRole || "U").slice(0, 1).toUpperCase()}
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 12, fontWeight: 500, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", color: T.ink }}>
                      {userEmail || userRole || "Профиль"}
                    </div>
                    <div style={{ fontSize: 10, color: T.ink50 }}>{userRole || "user"} · {formatQuota(quota)}</div>
                  </div>
                  <svg width={10} height={10} viewBox="0 0 10 10" fill="none" stroke={T.ink50} strokeWidth="1.6"><path d="M2 4l3 3 3-3"/></svg>
                </button>
                {showProfileMenu && (
                  <div style={popoverStyle({ bottom: 56, left: 10, right: 10 })}>
                    <PopItem href="/access?force=promo" onClick={() => setShowProfileMenu(false)}>
                      <span style={{ color: T.greenDark, fontWeight: 500 }}>Продлить доступ</span>
                    </PopItem>
                    <PopItem href="/profile" onClick={() => setShowProfileMenu(false)}>Профиль</PopItem>
                    <PopItem href="/privacy" onClick={() => setShowProfileMenu(false)}>Политика конфиденциальности</PopItem>
                    <div style={{ height: 1, background: T.ink05, margin: "4px 0" }} />
                    <PopItem href="/login" onClick={() => setShowProfileMenu(false)}>Выйти</PopItem>
                  </div>
                )}
              </div>
            </aside>
          )}
    </>
  );
}
