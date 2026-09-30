'use client'

import type { ReactNode } from 'react'
import { FONT_BODY, MS } from '../_theme'

function CenterMessage({ children }: { children: ReactNode }) {
  return (
    <div
      style={{
        minHeight: "100vh",
        background: MS.bg,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 24,
        fontFamily: FONT_BODY,
        color: MS.ink50,
        fontSize: 14,
      }}
    >
      {children}
    </div>
  );
}

export { CenterMessage }
