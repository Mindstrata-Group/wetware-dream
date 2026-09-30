"use client";

import type { Mode } from "../types";
import { T, btnReset } from "../theme";
import { dailyUnlockLabel } from "../utils";
import { modeHasMessages } from "../utils";

export function ModeSheet({ modes, activeIds, onToggle, onSelectAll, onClear, onClose, isMobile }: {
  modes: Mode[];
  activeIds: number[];
  onToggle: (id: number) => void | Promise<void>;
  onSelectAll: () => void;
  onClear: () => void;
  onClose: () => void;
  isMobile: boolean;
}) {
  const body = (
    <>
      <div style={{ padding: "14px 18px 10px", borderBottom: `1px solid ${T.ink10}`, display: "flex", alignItems: "center", justifyContent: "space-between", flexShrink: 0 }}>
        <div>
          <h2 style={{ fontFamily: T.fontHead, fontWeight: 600, fontSize: 18, margin: 0 }}>Настроить режимы</h2>
          <p style={{ fontSize: 13, color: T.ink50, margin: "3px 0 0" }}>Можно выбрать один режим или оставить несколько: Стратум сам переключится по задаче.</p>
        </div>
        <button onClick={onClose} style={{ ...btnReset, width: 32, height: 32, borderRadius: 8, color: T.ink50, fontSize: 18 }}>×</button>
      </div>
      <div style={{ padding: "8px 14px", borderBottom: `1px solid ${T.ink10}`, display: "flex", gap: 8, flexShrink: 0 }}>
        <button onClick={onSelectAll} style={{ ...btnReset, height: 30, padding: "0 12px", borderRadius: 8, border: `1px solid ${T.ink20}`, fontSize: 12, color: T.ink70 }}>Использовать все</button>
        <button onClick={onClear} style={{ ...btnReset, height: 30, padding: "0 12px", borderRadius: 8, border: `1px solid ${T.ink20}`, fontSize: 12, color: T.ink70 }}>Очистить выбор</button>
      </div>
      <div style={{ overflowY: "auto", padding: "6px 10px", flex: 1 }}>
        {modes.map(m => {
          const selected = activeIds.includes(m.id);
          const locked = !modeHasMessages(m);
          return (
            <button key={m.id} aria-label={m.name} onClick={() => void onToggle(m.id)} style={{
              ...btnReset, width: "100%", padding: "10px 12px", borderRadius: 12,
              marginBottom: 4, justifyContent: "flex-start", gap: 12, textAlign: "left",
              background: selected ? T.greenLight : "transparent",
              border: `1px solid ${selected ? T.green : "transparent"}`,
              opacity: locked ? 0.5 : 1, cursor: locked ? "not-allowed" : "pointer",
            }}>
              <div style={{ width: 38, height: 38, borderRadius: 10, background: selected ? "#fff" : T.surfaceSoft, border: `1px solid ${T.ink10}`, display: "flex", alignItems: "center", justifyContent: "center", color: selected ? T.greenDark : T.ink70, fontFamily: T.fontHead, fontWeight: 600, fontSize: 14, flexShrink: 0 }}>
                {m.name.charAt(0)}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 14, fontWeight: 500, color: selected ? T.greenDark : T.ink }}>{m.name}</div>
                {locked && <div style={{ fontSize: 11, color: T.red }}>Лимит · {dailyUnlockLabel()}</div>}
              </div>
              {selected && <span style={{ color: T.green, fontSize: 16 }}>✓</span>}
            </button>
          );
        })}
      </div>
    </>
  );

  if (!isMobile) {
    return (
      <div onClick={onClose} style={{ position: "fixed", inset: 0, background: "rgba(13,27,22,0.4)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 60 }}>
        <div onClick={e => e.stopPropagation()} style={{ width: 540, maxHeight: "82vh", background: T.surface, borderRadius: T.radius, boxShadow: "0 30px 80px -20px rgba(0,0,0,0.4)", display: "flex", flexDirection: "column" }}>
          {body}
        </div>
      </div>
    );
  }
  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, background: "rgba(13,27,22,0.4)", display: "flex", alignItems: "flex-end", zIndex: 60 }}>
      <div onClick={e => e.stopPropagation()} style={{ width: "100%", maxHeight: "88dvh", background: T.surface, borderTopLeftRadius: 18, borderTopRightRadius: 18, display: "flex", flexDirection: "column" }}>
        <div style={{ height: 4, width: 38, background: T.ink20, borderRadius: 2, margin: "10px auto 0" }} />
        {body}
      </div>
    </div>
  );
}
