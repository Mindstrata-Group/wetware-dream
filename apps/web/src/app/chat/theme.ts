// SOLID-split of chat/page.tsx 2026-05-31: styling constants and helpers.

export const T = {
  bg:          "var(--chat-bg)",
  surface:     "var(--chat-surface)",
  surfaceSoft: "var(--chat-surface-soft)",
  green:       "#1D9E75",
  greenDark:   "#0F6E56",
  greenLight:  "#E1F5EE",
  ink:         "var(--chat-ink)",
  ink70:       "var(--chat-ink-70)",
  ink50:       "var(--chat-ink-50)",
  ink20:       "var(--chat-ink-20)",
  ink10:       "var(--chat-ink-10)",
  ink05:       "var(--chat-ink-05)",
  red:         "var(--chat-red)",
  blue:        "var(--chat-blue)",
  infoBg:      "var(--chat-info-bg)",
  infoBd:      "var(--chat-info-bd)",
  fontHead:    '"Unbounded", system-ui, sans-serif',
  fontBody:    '"Golos Text", system-ui, sans-serif',
  radius:      16,
  radiusSm:    10,
};

export const btnReset: React.CSSProperties = {
  display: "inline-flex", alignItems: "center", justifyContent: "center",
  textDecoration: "none", cursor: "pointer", outline: "none",
  boxShadow: "none", fontFamily: T.fontBody, lineHeight: 1,
  border: "none", padding: 0, margin: 0, background: "transparent",
};

export function iconBtnStyle(opts: { active?: boolean; color?: string } = {}): React.CSSProperties {
  return {
    width: 32, height: 32, borderRadius: 8, border: "none",
    background: opts.active ? T.greenLight : "transparent",
    color: opts.color || (opts.active ? T.greenDark : T.ink70),
    cursor: "pointer", display: "inline-flex", alignItems: "center", justifyContent: "center",
    fontFamily: T.fontBody, fontSize: 13,
  };
}

export function popoverStyle(p: { top?: number; right?: number; bottom?: number; left?: number; w?: number }): React.CSSProperties {
  return {
    position: "absolute", top: p.top, right: p.right, bottom: p.bottom, left: p.left, width: p.w,
    background: T.surface, border: "1px solid " + T.ink10, borderRadius: 10,
    boxShadow: "0 10px 30px -10px rgba(0,0,0,0.2)", padding: 6, zIndex: 30,
    display: "flex", flexDirection: "column", gap: 1,
  };
}
