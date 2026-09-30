"use client";

export function ErrorDebugDetails({ debug }: { debug: unknown }) {
  if (!debug) return null;
  return (
    <details className="ms-info-box ms-markdown-result">
      <summary>Расширенный лог live AI</summary>
      <pre>{JSON.stringify(debug, null, 2)}</pre>
    </details>
  );
}
