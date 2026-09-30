'use client'

import { useEffect, useState } from 'react'
import type { ContentItem } from './_types'
import { StatusBadge, type SaveStatus } from '@/lib/useAutoSave'

function toBoolean(value: unknown, fallback = false) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'string') return value === 'true' || value === '1'
  return fallback
}

function BooleanField({
  ckey,
  label,
  hint,
  items,
  onSave,
  fallback = false,
}: {
  ckey: string;
  label: string;
  hint?: string;
  items: ContentItem[];
  onSave: (key: string, value: unknown) => Promise<void>;
  fallback?: boolean;
}) {
  const original = toBoolean(items.find((i) => i.key === ckey)?.value, fallback)
  const [draft, setDraft] = useState(original)
  const [status, setStatus] = useState<SaveStatus>({ kind: 'idle' })

  useEffect(() => { setDraft(original) }, [original])

  async function toggle() {
    const next = !draft
    setDraft(next)
    setStatus({ kind: 'saving' })
    try {
      await onSave(ckey, next)
      setStatus({ kind: 'saved' })
      window.setTimeout(() => setStatus((s) => (s.kind === 'saved' ? { kind: 'idle' } : s)), 2500)
    } catch (e) {
      setDraft(draft)
      setStatus({ kind: 'error', msg: String((e as Error).message || e) })
    }
  }

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      gap: 14,
      padding: '12px 14px',
      border: `1px solid ${draft !== original ? 'var(--accent)' : 'var(--line)'}`,
      borderRadius: 10,
      background: 'var(--card)',
      marginBottom: 12,
    }}>
      <div style={{ minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <label style={{ fontSize: 13, fontWeight: 600, color: 'var(--foreground)' }}>{label}</label>
          <StatusBadge status={status} />
        </div>
        {hint && <div style={{ marginTop: 4, fontSize: 12, color: 'var(--muted)', lineHeight: 1.45 }}>{hint}</div>}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={draft}
        onClick={() => { void toggle() }}
        style={{
          width: 52,
          height: 30,
          borderRadius: 999,
          border: `1px solid ${draft ? 'var(--accent)' : 'var(--line)'}`,
          background: draft ? 'var(--accent)' : 'var(--soft)',
          padding: 3,
          flexShrink: 0,
        }}
      >
        <span style={{
          display: 'block',
          width: 22,
          height: 22,
          borderRadius: '50%',
          background: '#fff',
          transform: draft ? 'translateX(20px)' : 'translateX(0)',
          transition: 'transform 0.16s ease',
        }} />
      </button>
    </div>
  )
}

export { BooleanField }
