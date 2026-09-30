"use client";

import { useEffect, useState } from "react";
import { exportLimitLabel } from "../utils";

export function ExportLimitSlider({
  value,
  onCommit,
}: {
  value: string;
  onCommit: (value: string) => void;
}) {
  const [draft, setDraft] = useState(value);
  const [inputText, setInputText] = useState(value === "1000001" ? "" : value);
  useEffect(() => {
    setDraft(value);
    setInputText(value === "1000001" ? "" : value);
  }, [value]);

  function commit(next = draft) {
    if (next !== value) onCommit(next);
  }

  function handleTextChange(raw: string) {
    setInputText(raw);
    if (raw === "") {
      setDraft("1000001");
    } else {
      const n = parseInt(raw, 10);
      if (!isNaN(n) && n >= 1) {
        setDraft(String(n));
      }
    }
  }

  function handleTextBlur() {
    const n = parseInt(inputText, 10);
    if (!inputText || isNaN(n) || n < 1) {
      // infinity
      setDraft("1000001");
      setInputText("");
      commit("1000001");
    } else {
      commit(String(n));
    }
  }

  return (
    <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <span>Лимит сообщений экспорта: {exportLimitLabel(draft)}</span>
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <input
          type="range" min="1" max="1000001" step="1000" value={draft}
          style={{ flex: 1 }}
          onChange={(e) => {
            const v = e.target.value;
            setDraft(v);
            setInputText(v === "1000001" ? "" : v);
          }}
          onPointerUp={(e) => commit(e.currentTarget.value)}
          onKeyUp={(e) => commit(e.currentTarget.value)}
          onBlur={(e) => commit(e.currentTarget.value)}
        />
        <input
          type="number"
          min="1"
          placeholder="∞"
          value={inputText}
          style={{ width: 90, padding: "4px 8px", border: "1px solid var(--line)", borderRadius: 8, fontSize: 13 }}
          onChange={(e) => handleTextChange(e.target.value)}
          onBlur={handleTextBlur}
          onKeyDown={(e) => { if (e.key === "Enter") handleTextBlur(); }}
        />
      </div>
      <small>Оставьте поле пустым для бесконечности.</small>
    </label>
  );
}
