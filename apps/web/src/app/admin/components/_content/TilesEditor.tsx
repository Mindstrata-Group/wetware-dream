'use client'

import { useEffect, useState } from 'react'
import { StatusBadge, useAutoSave } from '@/lib/useAutoSave'
import type { ContentItem, Tile } from './_types'
import { btnIcon } from './_styles'

function TilesEditor({
  ckey, items, onSave,
}: {
  ckey: string;
  items: ContentItem[];
  onSave: (key: string, value: unknown) => Promise<void>;
}) {
  const original = (items.find((i) => i.key === ckey)?.value as Tile[]) ?? [];
  const [draft, setDraft] = useState<Tile[]>(() => JSON.parse(JSON.stringify(original)));

  const status = useAutoSave({ ckey, value: draft, originalValue: original, onSave });

  // Reinitialise when original changes (after save).
  useEffect(() => {
    setDraft(JSON.parse(JSON.stringify(original)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items.find((i) => i.key === ckey)?.updatedAt]);

  const updateTile = (i: number, patch: Partial<Tile>) => {
    setDraft((d) => d.map((t, idx) => (idx === i ? { ...t, ...patch } : t)));
  };

  const moveUp = (i: number) => {
    if (i === 0) return;
    setDraft((d) => {
      const next = [...d];
      [next[i - 1], next[i]] = [next[i], next[i - 1]];
      return next;
    });
  };

  const moveDown = (i: number) => {
    setDraft((d) => {
      if (i === d.length - 1) return d;
      const next = [...d];
      [next[i], next[i + 1]] = [next[i + 1], next[i]];
      return next;
    });
  };

  const remove = (i: number) => {
    if (!confirm("Удалить плашку?")) return;
    setDraft((d) => d.filter((_, idx) => idx !== i));
  };

  const add = () => {
    const nextId = Math.max(0, ...draft.map((t) => t.id)) + 1;
    setDraft((d) => [...d, { id: nextId, title: "Новая плашка", body: "Описание..." }]);
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, marginBottom: 20 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
        <label style={{ fontSize: 13, fontWeight: 600, color: "var(--foreground)" }}>
          Плашки «Как это работает»
        </label>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <StatusBadge status={status} />
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            onClick={add}
            style={{
              padding: "6px 12px", borderRadius: 8, border: "1px solid var(--line)",
              background: "var(--card)", color: "var(--foreground)", fontSize: 12, fontWeight: 500,
              cursor: "pointer", fontFamily: "inherit",
            }}
          >
            + Добавить
          </button>
        </div>
      </div>
      {draft.map((tile, i) => (
        <div
          key={tile.id}
          style={{
            border: "1px solid var(--line)",
            borderRadius: 12,
            padding: 14,
            background: "var(--card)",
            display: "flex",
            flexDirection: "column",
            gap: 8,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span style={{
              width: 28, height: 28, borderRadius: "50%",
              background: "var(--soft)", border: "1px solid #1D9E75",
              display: "flex", alignItems: "center", justifyContent: "center",
              fontSize: 11, fontWeight: 700, color: "var(--foreground)",
            }}>
              {String(i + 1).padStart(2, "0")}
            </span>
            <input
              type="text"
              value={tile.title}
              onChange={(e) => updateTile(i, { title: e.target.value })}
              placeholder="Заголовок"
              style={{
                flex: 1, padding: "6px 10px", borderRadius: 6,
                border: "1px solid var(--line)", background: "var(--background)",
                color: "var(--foreground)", fontFamily: "inherit", fontSize: 13, fontWeight: 600,
              }}
            />
            <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => moveUp(i)} disabled={i === 0} style={btnIcon}>↑</button>
            <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => moveDown(i)} disabled={i === draft.length - 1} style={btnIcon}>↓</button>
            <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => remove(i)} style={{ ...btnIcon, color: "#b00020" }}>✕</button>
          </div>
          <textarea
            value={tile.body}
            onChange={(e) => updateTile(i, { body: e.target.value })}
            placeholder="Описание"
            rows={2}
            style={{
              padding: "8px 10px", borderRadius: 6,
              border: "1px solid var(--line)", background: "var(--background)",
              color: "var(--foreground)", fontFamily: "inherit", fontSize: 13, resize: "vertical",
            }}
          />
        </div>
      ))}
    </div>
  );
}

export { TilesEditor }
