"use client";

import { ModeSheet } from "../_components/ModeSheet";
import { HistorySheet } from "../_components/HistorySheet";
import { ProfileSheet } from "../_components/ProfileSheet";
import type { useChatPageController } from "../_hooks/useChatPageController";

export function ChatSheets({ controller }: { controller: ReturnType<typeof useChatPageController> }) {
  const {
    loading, error, errorDebug, modes, quota, modeId, selectedModeIds, dialogId, modeName, input,
    sending, completing, dialogCompleted, dialogAccessRequired, responseMode, userRole, userEmail,
    historySearch, setHistorySearch, orchestrationNotice, lockedModeNotice, summaryHistoryVisible, canChooseResponseMode, emailInitial, showModes, showHistory,
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
      {showModes && (
        <ModeSheet modes={modes} activeIds={selectedModeIds} onToggle={toggleModeSelection} onSelectAll={selectAllModes} onClear={clearModeSelection} onClose={() => setShowModes(false)} isMobile={mobile} />
      )}
      {showHistory && (
        <HistorySheet items={filteredHistory} currentDialogId={dialogId} onRestore={restoreDialog} onDelete={dialogIdToDelete => void deleteHistoryItem(dialogIdToDelete)} onTogglePin={togglePin} onRename={renameHistory} onClear={() => void clearAllHistory()} onClose={() => setShowHistory(false)} search={historySearch} setSearch={setHistorySearch} />
      )}
      {showProfile && compact && <ProfileSheet onClose={() => setShowProfile(false)} />}
    </>
  );
}
