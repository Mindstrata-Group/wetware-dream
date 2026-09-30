"use client";

import { T } from "../theme";

export function ErrorDebugDetails({ debug }: { debug: unknown }) {
  if (!debug) return null;
  return (
    <details style={{ marginTop: 8 }}>
      <summary style={{ fontSize: 12, color: T.ink50, cursor: "pointer" }}>Расширенный лог live AI</summary>
      <pre style={{ fontSize: 11, background: T.ink, color: T.greenLight, borderRadius: 8, padding: 10, overflow: "auto", marginTop: 6 }}>{JSON.stringify(debug, null, 2)}</pre>
    </details>
  );
}
