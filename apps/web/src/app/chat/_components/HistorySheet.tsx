"use client";

import { useEffect, useRef, useState } from "react";
import type { ChatHistoryItem } from "../types";
import { T, btnReset, iconBtnStyle, popoverStyle } from "../theme";
import { formatDate } from "../utils";
import { PopItem } from "./PopItem";

export function HistorySheet({ items, currentDialogId, onRestore, onDelete, onTogglePin, onRename, onClear, onClose, search, setSearch }: {
  items: ChatHistoryItem[];
  currentDialogId: number | null;
  onRestore: (item: ChatHistoryItem) => void;
  onDelete: (id: number) => void;
  onTogglePin: (id: number) => void;
  onRename: (id: number) => void;
  onClear: () => void;
  onClose: () => void;
  search: string;
  setSearch: (v: string) => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const menuWrapRef = useRef<HTMLDivElement | null>(null);
  const menuPopoverRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => { setMenuOpen(false); }, [search]);
  useEffect(() => {
    const onPointerDown = (event: PointerEvent) => {
      if (!menuOpen) return;
      const target = event.target as Node | null;
      if (menuWrapRef.current?.contains(target) || menuPopoverRef.current?.contains(target)) return;
      setMenuOpen(false);
    };
    window.addEventListener("pointerdown", onPointerDown);
    return () => window.removeEventListener("pointerdown", onPointerDown);
  }, [menuOpen]);
  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, background: "rgba(13,27,22,0.4)", display: "flex", alignItems: "flex-end", zIndex: 60 }}>
      <div onClick={e => e.stopPropagation()} style={{ width: "100%", maxHeight: "92dvh", background: T.surface, borderTopLeftRadius: 18, borderTopRightRadius: 18, display: "flex", flexDirection: "column" }}>
        <div style={{ height: 4, width: 38, background: T.ink20, borderRadius: 2, margin: "10px auto 0" }} />
        <div style={{ padding: "8px 16px 12px", borderBottom: `1px solid ${T.ink10}`, display: "flex", flexDirection: "column", gap: 10, flexShrink: 0, position: "relative" }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <h2 style={{ fontFamily: T.fontHead, fontWeight: 600, fontSize: 18, margin: 0 }}>История диалогов</h2>
            <div ref={menuWrapRef} style={{ display: "flex", gap: 4 }}>
              <button onClick={() => setMenuOpen(v => !v)} style={iconBtnStyle()} title="Меню">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke={T.ink50} strokeWidth="1.6"><circle cx="3" cy="8" r="1" /><circle cx="8" cy="8" r="1" /><circle cx="13" cy="8" r="1" /></svg>
              </button>
              <button onClick={onClose} style={{ ...btnReset, width: 32, height: 32, borderRadius: 8, color: T.ink50, fontSize: 18 }}>×</button>
            </div>
            {menuOpen && (
              <div ref={menuPopoverRef} style={popoverStyle({ top: 32, right: 56, w: 200 })}>
                <PopItem danger onClick={() => { onClear(); setMenuOpen(false); }}>Очистить историю</PopItem>
              </div>
            )}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8, height: 38, borderRadius: T.radiusSm, background: T.surfaceSoft, border: `1px solid ${T.ink10}`, padding: "0 12px" }}>
            <svg width={14} height={14} viewBox="0 0 16 16" fill="none" stroke={T.ink50} strokeWidth="1.6"><circle cx="7" cy="7" r="5"/><path d="M12 12l2.5 2.5"/></svg>
            <input value={search} onChange={e => { setMenuOpen(false); setSearch(e.target.value); }} placeholder="Поиск по чатам" style={{ flex: 1, border: "none", outline: "none", background: "transparent", fontFamily: T.fontBody, fontSize: 14, color: T.ink }} />
          </div>
        </div>
        <div style={{ overflowY: "auto", padding: "4px 12px 16px", flex: 1 }}>
          {items.length === 0 ? (
            <div style={{ padding: 20, textAlign: "center", color: T.ink50, fontSize: 13 }}>Пока нет сохранённых диалогов.</div>
          ) : items.map(item => (
            <div key={item.dialogId} onClick={() => setMenuOpen(false)} style={{ padding: "10px 4px", borderBottom: `1px solid ${T.ink05}`, position: "relative" }}>
              <button onClick={() => { onRestore(item); onClose(); }} style={{ ...btnReset, width: "100%", flexDirection: "column", alignItems: "flex-start", gap: 4, textAlign: "left" }}>
                <div style={{ display: "flex", justifyContent: "space-between", width: "100%", gap: 8 }}>
                  <span style={{ fontSize: 14, fontWeight: 600, color: item.dialogId === currentDialogId ? T.greenDark : T.ink, lineHeight: 1.3 }}>
                    {item.pinned ? "📌 " : ""}{item.title}
                  </span>
                  <span style={{ fontSize: 11, color: T.ink50, whiteSpace: "nowrap", flexShrink: 0 }}>{formatDate(item.updatedAt)}</span>
                </div>
                {item.modeName && <span style={{ fontSize: 12, padding: "2px 8px", borderRadius: 999, background: T.greenLight, color: T.greenDark }}>{item.modeName}</span>}
                {item.snippet && <span style={{ fontSize: 12, color: T.ink50, lineHeight: 1.4 }}>{item.snippet}</span>}
              </button>
              <div style={{ display: "flex", gap: 6, marginTop: 6 }}>
                <button onClick={() => onTogglePin(item.dialogId)} style={{ ...btnReset, height: 28, padding: "0 10px", borderRadius: 6, border: `1px solid ${T.ink20}`, fontSize: 11, color: T.ink70 }}>{item.pinned ? "Открепить" : "Закрепить"}</button>
                <button onClick={() => onRename(item.dialogId)} style={{ ...btnReset, height: 28, padding: "0 10px", borderRadius: 6, border: `1px solid ${T.ink20}`, fontSize: 11, color: T.ink70 }}>Переименовать</button>
                <button onClick={() => onDelete(item.dialogId)} style={{ ...btnReset, height: 28, padding: "0 10px", borderRadius: 6, border: `1px solid ${T.ink20}`, fontSize: 11, color: T.red }}>×</button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
