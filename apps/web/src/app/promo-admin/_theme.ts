'use client'

import type { CSSProperties } from 'react'

export const MS = {
  bg: "var(--background)",
  surface: "var(--card)",
  surfaceSoft: "var(--soft)",
  ink: "var(--foreground)",
  ink70: "var(--foreground)",
  ink50: "var(--muted)",
  ink20: "var(--line)",
  ink10: "var(--line)",
  ink05: "rgba(13,27,22,0.04)",
  // Brand accents: deliberately hard-coded, identical to the rest of the site.
  green: "#1D9E75",
  greenDark: "#0F6E56",
  // Theme-aware: pastel backgrounds must be green var(--soft) / red
  // var(--danger-bg), otherwise they glow as a white patch in the dark theme.
  greenLight: "var(--soft)",
  warn: "#C44545",
  warnLight: "var(--danger-bg)",
};

export const FONT_HEAD = '"Unbounded", system-ui, sans-serif';
export const FONT_BODY = '"Golos Text", system-ui, sans-serif';
export const FONT_MONO = 'ui-monospace, "JetBrains Mono", "SF Mono", Menlo, monospace';
