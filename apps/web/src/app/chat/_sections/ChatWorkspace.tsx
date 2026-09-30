"use client";
import Link from "next/link";
import { MessageContent } from "@/components/MessageContent";
import { T, btnReset, iconBtnStyle } from "../theme";
import { dailyUnlockLabel, displaySystemMessage, formatDate } from "../utils";
import { ErrorDebugDetails } from "../_components/ErrorDebugDetails";
import { TypingDots } from "../_components/TypingDots";
import type { useChatPageController } from "../_hooks/useChatPageController";
export function ChatWorkspace({ controller }: { controller: ReturnType<typeof useChatPageController> }) {
const {
loading, error, errorDebug, start, modes, quota, modeId, selectedModeIds, dialogId, modeName, input,
attachments, attachmentError, uploadingAttachment, dragActive, fileInputRef,
sending, completing, dialogCompleted, dialogAccessRequired, responseMode, userRole, userEmail, maxLinked, maxBotLink, startMaxLinkPolling,
historySearch, orchestrationNotice, lockedModeNotice, summaryHistoryVisible, canChooseResponseMode, emailInitial, showModes, showHistory,
showProfile, showSearch, showJumpToBottom, chatSearchQ, showHistMenu, showProfileMenu, hoverChatId,
editingHistoryId, editingHistoryTitle, idleDots, mobile, fold, desktop, compact, summaryMessages,
	chatScrollRef, textareaRef, attachTextarea, topSentinelRef, historyMenuRef, profileMenuRef, isComposingRef, pushClientLog, scrollChatToBottom, sendMessage,
	setChatMessageRef,
	completeDialog, handleInputChange, resetContext, restoreDialog, deleteHistoryItem, clearAllHistory, togglePin,
	renameHistory, saveHistoryRename, toggleModeSelection, selectAllModes, clearModeSelection, dailyLimitExhausted,
	messageLimit, messageLength, messageCharsRemaining, isMessageLimitNear, isMessageLimitReached,
	canSend, handleTextareaKeyDown, handleAttachmentInputChange, handleDrop, handleDragOver, handleDragLeave, removeAttachment,
	activeHistoryItem, filteredHistory, visibleConversation, quotaWarning, padH, headerH,
	chatSearchMatchIds, chatSearchActiveMessageId,
bottomNavH, setInput, setResponseMode, setShowModes, setShowHistory, setShowProfile, setShowSearch,
setChatSearchQ, setShowHistMenu, setShowProfileMenu, setHoverChatId, setEditingHistoryId,
setEditingHistoryTitle, setIdleDots,
} = controller;
const { beforeId, conversationMessages, thinkingLabel, currentModeLabel } = controller;
const isGuestSession = userRole === "guest" || (!userRole.trim() && !userEmail.trim());
const searchHighlight = chatSearchQ.trim();
const chatSearchMatchIdSet = new Set(chatSearchMatchIds);
return <>
<section
onDrop={handleDrop}
onDragOver={handleDragOver}
onDragLeave={handleDragLeave}
style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", position: "relative" }}
>
{dragActive && (
<div aria-live="polite" style={{ position: "absolute", inset: 10, zIndex: 20, borderRadius: 14, border: `2px dashed ${T.green}`, background: "rgba(225,245,238,0.88)", color: T.greenDark, display: "flex", alignItems: "center", justifyContent: "center", fontSize: 15, fontWeight: 600, pointerEvents: "none" }}>
Отпустите файл, чтобы прикрепить его к сообщению
</div>
)}
{loading ? (
<div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", color: T.ink50, fontSize: 14 }}>Загрузка...</div>
) : error ? (
<div style={{ flex: 1, padding: 20, display: "flex", flexDirection: "column", gap: 10 }}>
<div style={{ padding: "14px 16px", borderRadius: T.radiusSm, background: "#FBF0F0", border: `1px solid #EFB9B9` }}>
<strong style={{ color: T.red, fontSize: 14 }}>Ошибка</strong>
<div style={{ fontSize: 13, color: T.ink70, marginTop: 4 }}>{error}</div>
<ErrorDebugDetails debug={errorDebug} />
<button onClick={() => void start()} style={{ ...btnReset, marginTop: 10, height: 36, padding: "0 16px", borderRadius: 8, background: T.green, color: "#fff", fontSize: 13, fontWeight: 500 }}>Повторить</button>
</div>
</div>
) : (
<>
<div className="ms-chat-feed" ref={chatScrollRef} style={{ flex: 1, minHeight: 0, overflowY: "auto", padding: desktop ? "20px max(8%, 24px)" : `14px ${padH}px`, display: "flex", flexDirection: "column", gap: 10, background: `${T.bg} radial-gradient(rgba(122,154,142,0.12) 1px, transparent 1px)`, backgroundSize: "14px 14px", animation: idleDots ? "ms-dots-drift 16s ease-in-out infinite" : "none" }}>
<div style={{ alignSelf: "center", padding: "3px 10px", borderRadius: 12, background: T.surface, border: `1px solid ${T.ink10}`, fontSize: 11, color: T.ink50 }}>сегодня</div>
{beforeId && <div ref={topSentinelRef} style={{ height: 1 }} />}
{conversationMessages.length === 0 && (
<div style={{ alignSelf: "flex-start", maxWidth: "84%", minWidth: 0, display: "flex", flexDirection: "column", gap: 4 }}>
<div style={{ fontSize: 11, color: T.ink50, marginBottom: 4, display: "flex", gap: 6, alignItems: "center" }}>
<span style={{ color: T.greenDark, fontWeight: 500 }}>Стратум</span>
<time style={{ color: T.ink50 }}>{formatDate(new Date().toISOString())}</time>
</div>
<div style={{ padding: "9px 13px", borderRadius: 14, borderBottomLeftRadius: 4, background: T.surface, color: T.ink, fontSize: 14, lineHeight: 1.5, border: `1px solid ${T.ink10}`, maxWidth: "100%", overflowX: "auto", overflowWrap: "break-word", wordBreak: "break-word" }}>
<MessageContent content={modes.find(m => m.id === modeId)?.welcomeMessage || "Опишите ситуацию обычными словами: что происходит, что уже пробовали и какой результат нужен. Если режимов выбрано несколько, я сам переключусь по задаче."} />
</div>
</div>
)}
{visibleConversation.map((msg, index) => {
const prev = visibleConversation[index - 1];
const isUser = msg.role === "user";
const assistantModeName = !isUser && msg.role === "assistant" ? (msg.modeName || "") : "";
const showMeta = !prev || prev.role !== msg.role || (!isUser && assistantModeName && prev.modeName !== assistantModeName);
const isSearchMatch = chatSearchMatchIdSet.has(msg.id);
const isSearchActive = msg.id === chatSearchActiveMessageId;
if (msg.role === "system") {
return (
<div key={`${msg.role}-${msg.id}-${msg.createdAt}`} ref={node => setChatMessageRef(msg.id, node)} data-chat-message-id={msg.id} data-chat-search-active={isSearchActive ? "true" : undefined} title={formatDate(msg.createdAt)} style={{ alignSelf: "center", maxWidth: "min(88%, 560px)", padding: "4px 10px", borderRadius: 999, border: `1px solid ${isSearchActive ? T.green : T.ink10}`, background: isSearchActive ? T.greenLight : T.surfaceSoft, color: T.ink50, fontSize: 11, lineHeight: 1.35, textAlign: "center", boxShadow: isSearchActive ? "0 0 0 2px rgba(29,158,117,0.12)" : "none" }}>
{displaySystemMessage(msg.content)}
</div>
);
}
return (
<div key={`${msg.role}-${msg.id}-${msg.createdAt}`} ref={node => setChatMessageRef(msg.id, node)} data-chat-message-id={msg.id} data-chat-search-active={isSearchActive ? "true" : undefined} style={{ display: "flex", flexDirection: "column", alignItems: isUser ? "flex-end" : "flex-start", maxWidth: "84%", minWidth: 0, alignSelf: isUser ? "flex-end" : "flex-start", outline: isSearchActive ? `2px solid ${T.green}` : "none", outlineOffset: 3, borderRadius: 16 }}>
{showMeta && (
<div style={{ fontSize: 11, color: T.ink50, marginBottom: 4, display: "flex", gap: 6, alignItems: "center" }}>
{!isUser && <span style={{ color: T.greenDark, fontWeight: 500 }}>Стратум</span>}
{assistantModeName && <span style={{ color: "#7A5D2E", fontWeight: 500 }}>{assistantModeName}</span>}
<time style={{ color: T.ink50 }}>{formatDate(msg.createdAt)}</time>
{isUser && <span style={{ color: T.ink70 }}>Вы</span>}
</div>
)}
<div style={{ padding: "9px 13px", borderRadius: 14, borderBottomRightRadius: isUser ? 4 : 14, borderBottomLeftRadius: isUser ? 14 : 4, background: isUser ? T.green : T.surface, color: isUser ? "#fff" : T.ink, fontSize: 14, lineHeight: 1.5, border: isUser ? (isSearchMatch ? `1px solid ${T.greenDark}` : "none") : `1px solid ${isSearchActive ? T.green : T.ink10}`, maxWidth: "100%", overflowX: "auto", overflowWrap: "break-word", wordBreak: "break-word" }}>
<MessageContent content={msg.content} highlight={searchHighlight} />
</div>
</div>
);
})}
{sending && (
<div style={{ alignSelf: "flex-start", maxWidth: "84%", display: "flex", flexDirection: "column", gap: 4 }}>
<div style={{ fontSize: 11, color: T.ink50 }}><span style={{ color: T.greenDark, fontWeight: 500 }}>Стратум</span> · {thinkingLabel}</div>
<div style={{ padding: "4px 13px", borderRadius: 14, borderBottomLeftRadius: 4, background: T.surface, border: `1px solid ${T.ink10}` }}>
<TypingDots />
</div>
</div>
)}
{summaryHistoryVisible && summaryMessages.length > 0 && summaryMessages.slice(-1).map(msg => (
<div key={`summary-${msg.id}`} style={{ alignSelf: "flex-start", maxWidth: "88%", minWidth: 0, display: "flex", flexDirection: "column", gap: 4 }}>
<div style={{ fontSize: 11, color: T.ink50 }}><span style={{ color: T.greenDark, fontWeight: 500 }}>Стратум</span> · итог · <time>{formatDate(msg.createdAt)}</time></div>
<div style={{ padding: "10px 13px", borderRadius: 14, borderBottomLeftRadius: 4, background: T.surface, border: `1px solid ${T.green}`, maxWidth: "100%", overflowX: "auto", overflowWrap: "break-word", wordBreak: "break-word" }}>
<div style={{ fontSize: 12, color: T.greenDark, fontWeight: 500, marginBottom: 6, display: "flex", alignItems: "center", gap: 6 }}>
<span style={{ width: 6, height: 6, borderRadius: "50%", background: T.green }} />Итог диалога
</div>
<MessageContent content={msg.content} />
</div>
</div>
))}
{orchestrationNotice && (
<div style={{ alignSelf: "stretch", display: "flex", alignItems: "flex-start", gap: 10, padding: "10px 12px", borderRadius: 10, background: T.infoBg, border: `1px solid ${T.infoBd}`, color: T.blue, fontSize: 12, lineHeight: 1.45, animation: "ms-orch-in 280ms ease" }}>
<div style={{ width: 8, height: 8, borderRadius: "50%", background: T.blue, marginTop: 5, flexShrink: 0 }} />
<div><span style={{ fontWeight: 600 }}>Стратум:</span> {orchestrationNotice}</div>
</div>
)}
</div>
{/* ── COMPOSER ── */}
{dialogAccessRequired || dailyLimitExhausted ? (
<div style={{ padding: `12px ${padH}px ${mobile ? 16 : 20}px`, background: T.surface, borderTop: `1px solid ${T.ink10}`, flexShrink: 0, display: "flex", flexDirection: "column", gap: 10 }}>
<div style={{ padding: "14px 16px", borderRadius: 12, background: "#FBECEC", border: `1px solid ${T.red}`, display: "flex", gap: 12, alignItems: "flex-start" }}>
<div style={{ width: 24, height: 24, borderRadius: "50%", background: T.red, color: "#fff", display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0, fontWeight: 700, fontFamily: T.fontHead, fontSize: 14 }}>!</div>
<div style={{ flex: 1, minWidth: 0 }}>
<div style={{ fontSize: 14, fontWeight: 600, color: T.red, lineHeight: 1.3 }}>
{dialogAccessRequired ? "Доступ к режиму этого диалога закончился." : "Дневной лимит сообщений исчерпан"}
</div>
<div style={{ fontSize: 12, color: T.ink70, marginTop: 4, lineHeight: 1.5 }}>
{dialogAccessRequired
? "Историю можно читать, но продолжить диалог получится после продления доступа."
: <span>Лимит обновится через <b style={{ color: T.ink, fontVariantNumeric: "tabular-nums" }}>{dailyUnlockLabel()}</b>, либо продлите доступ прямо сейчас.</span>
}
</div>
</div>
</div>
<div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
<Link href="/access?force=promo" style={{ ...btnReset, height: 44, padding: "0 22px", borderRadius: T.radiusSm, background: T.green, color: "#fff", fontSize: 14, fontWeight: 600, flex: mobile ? 1 : "0 0 auto", justifyContent: "center" }}>Продлить доступ</Link>
{dailyLimitExhausted && !isGuestSession && !maxLinked && maxBotLink && (
<a href={maxBotLink} target="_blank" rel="noopener noreferrer" onClick={startMaxLinkPolling} className="ms-max-notify-btn" style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", height: 44, padding: "0 18px", borderRadius: T.radiusSm, border: `1.5px solid ${T.green}`, background: T.greenLight, color: T.greenDark, fontSize: 13, fontWeight: 600, textDecoration: "none", flex: mobile ? 1 : "0 0 auto", gap: 6 }}>
  <span>🔔</span> Включить уведомления → +{Math.max(2, Math.ceil((quota?.limit ?? 10) * 0.1))} сообщений сейчас
</a>
)}
{dailyLimitExhausted && isGuestSession && (
<Link href="/login?next=/chat" style={{ ...btnReset, height: 44, padding: "0 16px", borderRadius: T.radiusSm, border: `1px solid ${T.ink20}`, color: T.ink70, fontSize: 14, flex: mobile ? 1 : "0 0 auto", justifyContent: "center" }}>Войти и сохранить историю</Link>
)}
{!mobile && (
<button onClick={completeDialog} disabled={!dialogId || !modeId || completing} style={{ ...btnReset, height: 44, padding: "0 16px", borderRadius: T.radiusSm, border: `1px solid ${T.ink20}`, color: T.ink70, fontSize: 14 }}>Завершить и получить итог</button>
)}
</div>
</div>
) : (
        <div style={{ padding: `8px ${padH}px ${mobile ? 12 : 16}px`, background: T.surface, borderTop: `1px solid ${T.ink10}`, display: "flex", flexDirection: "column", gap: 8, flexShrink: 0 }}>
          {(isMessageLimitReached || isMessageLimitNear) && (
            <div style={{ display: "flex", justifyContent: "flex-end", fontSize: 11 }}>
              {isMessageLimitReached ? (
                <div role="status" aria-live="polite" style={{ color: T.red, fontWeight: 600 }}>Сообщение слишком длинное — сократите текст</div>
              ) : (
                <div role="status" aria-live="polite" style={{ color: "#F59E0B", fontWeight: 600 }}>Текст почти достиг лимита</div>
              )}
            </div>
          )}
          <div style={{ display: "flex", gap: 6, alignItems: "center", flexWrap: "wrap", position: "sticky", bottom: 0 }}>
<button onClick={() => setShowModes(true)} title="Выбор режимов" style={{ ...btnReset, height: 30, padding: "0 12px", borderRadius: 8, border: `1px solid ${T.ink20}`, background: T.surfaceSoft, color: T.ink70, fontSize: 12, gap: 6, display: mobile ? "none" : "inline-flex" }}>
<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6"><path d="M3 3v10a1 1 0 001 1h9V4a1 1 0 00-1-1H3z"/><path d="M3 11h10"/></svg>
{currentModeLabel || "Базовый ИИ"}
</button>
{desktop && dialogId && (
<>
<button onClick={() => togglePin(dialogId)} style={iconBtnStyle({ active: activeHistoryItem?.pinned })} title="Закрепить">📌</button>
<button onClick={() => renameHistory(dialogId)} style={iconBtnStyle()} title="Переименовать">
<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M11 2l3 3-8 8H3v-3l8-8z"/></svg>
</button>
<button onClick={() => setShowSearch(v => !v)} style={iconBtnStyle({ active: showSearch })} title="Поиск по чату и истории">
<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6"><circle cx="7" cy="7" r="5"/><path d="M12 12l3 3"/></svg>
</button>
</>
)}
<button onClick={completeDialog} disabled={!dialogId || !modeId || dialogCompleted || completing} style={{ ...btnReset, height: 30, padding: "0 12px", borderRadius: 8, border: `1px solid ${T.ink20}`, background: T.greenLight, color: T.greenDark, fontSize: 12, fontWeight: 500 }}>
{completing ? "Готовим итог…" : "Завершить и получить итог"}
</button>
{dialogCompleted && <span style={{ fontSize: 12, color: T.ink50 }}>Итог сохранён. Напишите, чтобы начать новый диалог.</span>}
</div>
{showJumpToBottom && (
<button onClick={() => scrollChatToBottom("smooth")} title="Вниз к последним сообщениям" style={{ ...btnReset, position: "absolute", right: mobile ? 12 : 20, bottom: mobile ? 124 : 100, width: 38, height: 38, borderRadius: "50%", background: T.surface, border: `1px solid ${T.ink20}`, color: T.greenDark, boxShadow: "0 6px 14px rgba(0,0,0,0.08)", zIndex: 5 }}>
↓
</button>
)}
{attachments.length > 0 && (
<div aria-label="Прикреплённые файлы" style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
{attachments.map(file => (
<span key={file.id} style={{ minHeight: 30, maxWidth: "100%", display: "inline-flex", alignItems: "center", gap: 7, padding: "5px 8px", borderRadius: 8, border: `1px solid ${T.ink10}`, background: T.surfaceSoft, color: T.ink70, fontSize: 12 }}>
<svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M5 2h4l3 3v9H5z"/><path d="M9 2v3h3"/><path d="M7 9h3M7 12h2"/></svg>
<span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", maxWidth: mobile ? 170 : 260 }}>{file.fileName}</span>
<span style={{ color: T.ink50, flexShrink: 0 }}>{Math.max(1, Math.round(file.sizeBytes / 1024))} КБ</span>
{file.annotationStatus === "annotated" && <span style={{ color: T.greenDark, flexShrink: 0 }}>сжато</span>}
<button type="button" onClick={() => removeAttachment(file.id)} title={`Убрать ${file.fileName}`} style={{ ...btnReset, width: 18, height: 18, borderRadius: 6, color: T.ink50, flexShrink: 0 }}>×</button>
</span>
))}
</div>
)}
	{attachmentError && <div role="alert" style={{ color: T.red, fontSize: 12, lineHeight: 1.35 }}>{attachmentError}</div>}
	{quotaWarning && (
	<div role="status" aria-live="polite" style={{ display: "flex", gap: 9, alignItems: "flex-start", padding: "10px 12px", borderRadius: 10, border: `1px solid ${T.infoBd}`, background: T.infoBg, color: T.ink70, fontSize: 12, lineHeight: 1.45 }}>
	<span aria-hidden="true" style={{ width: 8, height: 8, borderRadius: "50%", background: T.blue, marginTop: 5, flexShrink: 0 }} />
	<span><b style={{ color: T.ink }}>{quotaWarning.title}</b><br />{quotaWarning.body}</span>
	</div>
	)}
	<div style={{ display: "flex", alignItems: "flex-end", gap: 8 }}>
<input ref={fileInputRef} type="file" accept=".txt,.md,.doc,.docx,text/plain,text/markdown,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/msword" multiple onChange={handleAttachmentInputChange} style={{ display: "none" }} />
<button type="button" onClick={() => fileInputRef.current?.click()} disabled={uploadingAttachment || sending} title="Прикрепить файл" style={{ ...btnReset, width: 44, height: 44, borderRadius: "50%", border: `1px solid ${T.ink20}`, background: uploadingAttachment ? T.greenLight : T.surfaceSoft, color: uploadingAttachment ? T.greenDark : T.ink70, flexShrink: 0 }}>
{uploadingAttachment ? (
<span style={{ fontSize: 12, fontWeight: 600 }}>...</span>
) : (
<svg width={17} height={17} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M13 7.5l-5.1 5.1a3.2 3.2 0 01-4.5-4.5l5.7-5.7a2.1 2.1 0 013 3l-5.7 5.7a1 1 0 01-1.4-1.4l5.1-5.1"/></svg>
)}
</button>
            <textarea
              ref={attachTextarea}
              className="ms-chat-input"
              value={input}
              data-length={messageLength}
              inputMode="text"
              enterKeyHint="send"
              autoCapitalize="sentences"
              autoCorrect="on"
              spellCheck
              onChange={(e) => {
                setIdleDots(false);
                pushClientLog('change', { composing: isComposingRef.current, domLen: e.currentTarget.value.length });
                handleInputChange(e.currentTarget.value);
              }}
              onCompositionStart={() => {
                isComposingRef.current = true;
                pushClientLog('compositionStart', {});
              }}
              onCompositionEnd={(e) => {
                isComposingRef.current = false;
                const val = e.currentTarget.value;
                pushClientLog('compositionEnd', { domLen: val.length });
                setIdleDots(false);
                handleInputChange(val);
              }}
              onBlur={(e) => {
                const wasComposing = isComposingRef.current;
                isComposingRef.current = false;
                pushClientLog('blur', { wasComposing, domLen: e.currentTarget.value.length });
                if (e.currentTarget.value !== input) handleInputChange(e.currentTarget.value);
              }}
              onKeyDown={handleTextareaKeyDown}
              placeholder="Напишите сообщение"
              rows={1}
            />
<button onClick={() => canSend && void sendMessage(textareaRef.current?.value || input)} disabled={!canSend} style={{ ...btnReset, width: 44, height: 44, borderRadius: "50%", background: canSend ? T.green : T.ink20, color: "#fff", flexShrink: 0, fontSize: 18, transition: "background 0.15s" }} title="Отправить">
<svg width={16} height={16} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"><path d="M8 13V3M3 8l5-5 5 5"/></svg>
</button>
</div>
</div>
)}
</>
)}
</section>
</>;
}
