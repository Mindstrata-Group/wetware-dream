'use client'

import { useEffect, useState } from 'react'
import type { ContentItem } from './_types'
import { StatusBadge, useAutoSave } from '@/lib/useAutoSave'

function StringField({
  ckey, label, placeholder, items, onSave, multiline = false, rows = 3,
}: {
  ckey: string;
  label: string;
  placeholder?: string;
  items: ContentItem[];
  onSave: (key: string, value: unknown) => Promise<void>;
  multiline?: boolean;
  rows?: number;
}) {
  const original = (items.find((i) => i.key === ckey)?.value as string | undefined) ?? "";
  const [draft, setDraft] = useState<string>(original);

  // If items were updated (after a refetch), sync the draft.
  useEffect(() => { setDraft(original); }, [original]);

  const status = useAutoSave({ ckey, value: draft, originalValue: original, onSave });
  const dirty = draft !== original;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 6, marginBottom: 16 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
        <label style={{ fontSize: 12, fontWeight: 600, color: "var(--muted)" }}>{label}</label>
        <StatusBadge status={status} />
      </div>
      {multiline ? (
        <textarea
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={placeholder}
          rows={rows}
          style={{
            padding: "8px 12px", borderRadius: 8,
            border: `1px solid ${dirty ? "var(--accent)" : "var(--line)"}`,
            background: "var(--card)", color: "var(--foreground)",
            fontFamily: "inherit", fontSize: 14, resize: "vertical",
          }}
        />
      ) : (
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={placeholder}
          style={{
            padding: "8px 12px", borderRadius: 8,
            border: `1px solid ${dirty ? "var(--accent)" : "var(--line)"}`,
            background: "var(--card)", color: "var(--foreground)",
            fontFamily: "inherit", fontSize: 14,
          }}
        />
      )}
    </div>
  );
}

export { StringField }
