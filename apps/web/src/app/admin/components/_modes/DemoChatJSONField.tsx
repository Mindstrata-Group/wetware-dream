"use client";

import { useEffect, useState } from "react";

export function DemoChatJSONField({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const formatInitial = (nextValue: string) => {
    const trimmed = nextValue.trim();
    if (!trimmed) return "";
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2);
    } catch {
      return nextValue;
    }
  };

  const [local, setLocal] = useState(() => formatInitial(value));
  const [parseError, setParseError] = useState("");

  useEffect(() => {
    setLocal(formatInitial(value));
    setParseError("");
  }, [value]);

  const errorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error || "JSON parse error");

  const onLocalChange = (next: string) => {
    setLocal(next);
    if (!next.trim()) {
      setParseError("");
      onChange(next);
      return;
    }
    try {
      JSON.parse(next);
      setParseError("");
      onChange(next);
    } catch (error: unknown) {
      setParseError(errorMessage(error));
    }
  };

  const prettify = () => {
    if (!local.trim()) return;
    try {
      setLocal(JSON.stringify(JSON.parse(local), null, 2));
      setParseError("");
    } catch (error: unknown) {
      setParseError(errorMessage(error));
    }
  };

  return (
    <label style={{ fontWeight: 500 }}>
      Демо-диалог (raw JSON, показывается на главной)
      <div style={{ display: "flex", gap: 8, alignItems: "center", margin: "4px 0 6px" }}>
        <button
          type="button"
          onClick={prettify}
          disabled={!local.trim() || !!parseError}
          style={{
            padding: "4px 10px",
            borderRadius: 6,
            border: "1px solid var(--line)",
            background: "var(--card)",
            color: "var(--foreground)",
            fontSize: 11,
            cursor: "pointer",
            fontFamily: "inherit",
            opacity: !local.trim() || parseError ? 0.5 : 1,
          }}
        >
          ↺ Форматировать
        </button>
        {parseError ? (
          <span style={{ fontSize: 11, color: "#b00020" }}>✗ JSON некорректен: {parseError}</span>
        ) : local.trim() ? (
          <span style={{ fontSize: 11, color: "#1D9E75" }}>✓ JSON валиден</span>
        ) : (
          <span style={{ fontSize: 11, color: "var(--muted)" }}>пусто — на главной режим не будет показан</span>
        )}
      </div>
      <textarea
        rows={10}
        value={local}
        onChange={(event) => onLocalChange(event.target.value)}
        placeholder='[{"role":"user","text":"..."},{"role":"assistant","text":"..."}]'
        style={{
          width: "100%",
          fontFamily: '"JetBrains Mono", "SF Mono", Menlo, ui-monospace, monospace',
          fontSize: 12,
          border: `1px solid ${parseError ? "#b00020" : "var(--line)"}`,
          borderRadius: 6,
          padding: "8px 10px",
          background: "var(--card)",
          color: "var(--foreground)",
          resize: "vertical",
        }}
      />
      <small>
        Массив объектов <code>{`{role, text}`}</code> или другой формат — бэкенд хранит как jsonb. Битый JSON сохранить не дадим: и фронт, и Postgres его отбракуют.
      </small>
    </label>
  );
}
