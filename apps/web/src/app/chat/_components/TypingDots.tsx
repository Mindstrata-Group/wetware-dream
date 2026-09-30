"use client";

import { T } from "../theme";

export function TypingDots() {
  return (
    <div style={{ display: "flex", gap: 4, padding: "10px 2px", alignItems: "center" }}>
      {[0, 1, 2].map(i => (
        <span key={i} style={{ width: 6, height: 6, borderRadius: "50%", background: T.ink50, display: "inline-block", animation: `ms-typ 1.2s ${i * 0.15}s infinite` }} />
      ))}
    </div>
  );
}
