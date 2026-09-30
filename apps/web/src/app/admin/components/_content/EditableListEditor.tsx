'use client'

import { useEffect, useState } from 'react'
import type { ContentItem, EditableListField } from './_types'
import { btnIcon } from './_styles'
import { useAutoSave, StatusBadge } from '@/lib/useAutoSave'

function EditableListEditor<T extends object>({
  ckey, label, items, onSave, fallback, fields, makeNew,
}: {
  ckey: string;
  label: string;
  items: ContentItem[];
  onSave: (key: string, value: unknown) => Promise<void>;
  fallback: T[];
  fields: EditableListField[];
  makeNew: (nextIndex: number) => T;
}) {
  const original = (items.find((i) => i.key === ckey)?.value as T[] | undefined) ?? fallback;
  const [draft, setDraft] = useState<T[]>(() => JSON.parse(JSON.stringify(original)));
  const status = useAutoSave({ ckey, value: draft, originalValue: original, onSave });

  useEffect(() => {
    setDraft(JSON.parse(JSON.stringify(original)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items.find((i) => i.key === ckey)?.updatedAt]);

  const update = (row: number, field: string, value: string) => {
    setDraft((d) => d.map((item, idx) => (idx === row ? { ...item, [field]: value } : item)));
  };
  const move = (row: number, dir: -1 | 1) => {
    setDraft((d) => {
      const nextIndex = row + dir;
      if (nextIndex < 0 || nextIndex >= d.length) return d;
      const next = [...d];
      [next[row], next[nextIndex]] = [next[nextIndex], next[row]];
      return next;
    });
  };
  const remove = (row: number) => {
    if (!confirm("Удалить элемент?")) return;
    setDraft((d) => d.filter((_, idx) => idx !== row));
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, marginBottom: 20 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
        <label style={{ fontSize: 13, fontWeight: 600, color: "var(--foreground)" }}>{label}</label>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <StatusBadge status={status} />
          <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => setDraft((d) => [...d, makeNew(d.length + 1)])} style={{ padding: "6px 12px", borderRadius: 8, border: "1px solid var(--line)", background: "var(--card)", color: "var(--foreground)", fontSize: 12, fontWeight: 500, cursor: "pointer", fontFamily: "inherit" }}>+ Добавить</button>
        </div>
      </div>
      {draft.map((row, i) => (
        <div key={i} style={{ border: "1px solid var(--line)", borderRadius: 12, padding: 14, background: "var(--card)", display: "flex", flexDirection: "column", gap: 8 }}>
          <div style={{ display: "flex", justifyContent: "space-between", gap: 8, alignItems: "center" }}>
            <strong style={{ fontSize: 12, color: "var(--muted)" }}>#{i + 1}</strong>
            <div style={{ display: "flex", gap: 6 }}>
              <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => move(i, -1)} disabled={i === 0} style={btnIcon}>↑</button>
              <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => move(i, 1)} disabled={i === draft.length - 1} style={btnIcon}>↓</button>
              <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => remove(i)} style={{ ...btnIcon, color: "#b00020" }}>✕</button>
            </div>
          </div>
          {fields.map((field) => (
            <label key={field.key} style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 11, color: "var(--muted)", fontWeight: 600 }}>
              {field.label}
              {field.multiline ? (
                <textarea value={String((row as Record<string, unknown>)[field.key] ?? "")} onChange={(e) => update(i, field.key, e.target.value)} placeholder={field.placeholder} rows={2} style={{ padding: "8px 10px", borderRadius: 6, border: "1px solid var(--line)", background: "var(--background)", color: "var(--foreground)", fontFamily: "inherit", fontSize: 13, resize: "vertical" }} />
              ) : (
                <input value={String((row as Record<string, unknown>)[field.key] ?? "")} onChange={(e) => update(i, field.key, e.target.value)} placeholder={field.placeholder} style={{ padding: "6px 10px", borderRadius: 6, border: "1px solid var(--line)", background: "var(--background)", color: "var(--foreground)", fontFamily: "inherit", fontSize: 13 }} />
              )}
            </label>
          ))}
        </div>
      ))}
    </div>
  );
}

export { EditableListEditor }
