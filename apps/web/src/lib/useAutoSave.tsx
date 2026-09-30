'use client'

import { useEffect, useRef, useState } from 'react'

export type SaveStatus =
  | { kind: 'idle' }
  | { kind: 'pending' }     // debounce still running
  | { kind: 'saving' }      // POST in flight
  | { kind: 'saved' }       // success
  | { kind: 'error'; msg: string }

const DEFAULT_DELAY_MS = 5000

// useAutoSave tracks changes of `value` and after `delayMs` of silence
// (5 s by default) makes a POST via `onSave`.
// Returns a status for showing an indicator.
//
// Deep equality check via JSON.stringify (for arrays/objects).
export function useAutoSave<T>(opts: {
  ckey: string
  value: T
  originalValue: T
  onSave: (key: string, value: T) => Promise<void>
  delayMs?: number
}): SaveStatus {
  const { ckey, value, originalValue, onSave, delayMs = DEFAULT_DELAY_MS } = opts
  const [status, setStatus] = useState<SaveStatus>({ kind: 'idle' })
  const lastSavedRef = useRef<string>(JSON.stringify(originalValue))
  const inFlight = useRef(false)

  // Reset the baseline when a fresh originalValue is loaded from outside
  // (e.g. after a fetch on every items refresh).
  useEffect(() => {
    lastSavedRef.current = JSON.stringify(originalValue)
  }, [originalValue])

  useEffect(() => {
    const current = JSON.stringify(value)
    if (current === lastSavedRef.current) {
      // Matches the last saved value -> switch to idle (or keep 'saved').
      if (status.kind === 'pending' || status.kind === 'saving') {
        setStatus({ kind: 'idle' })
      }
      return
    }
    if (inFlight.current) return

    setStatus({ kind: 'pending' })
    const id = window.setTimeout(async () => {
      if (JSON.stringify(value) === lastSavedRef.current) return
      inFlight.current = true
      setStatus({ kind: 'saving' })
      try {
        await onSave(ckey, value)
        lastSavedRef.current = JSON.stringify(value)
        setStatus({ kind: 'saved' })
        window.setTimeout(() => {
          setStatus((s) => (s.kind === 'saved' ? { kind: 'idle' } : s))
        }, 2500)
      } catch (e) {
        setStatus({ kind: 'error', msg: String((e as Error).message || e) })
      } finally {
        inFlight.current = false
      }
    }, delayMs)
    return () => window.clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(value), ckey])

  return status
}

export function StatusBadge({ status }: { status: SaveStatus }) {
  const base: React.CSSProperties = {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 5,
    fontSize: 11,
    fontWeight: 500,
    padding: '2px 8px',
    borderRadius: 5,
    fontFamily: 'inherit',
    transition: 'opacity 200ms',
  }
  if (status.kind === 'idle') return null
  if (status.kind === 'pending') return <span style={{ ...base, color: 'var(--muted)', background: 'var(--soft)' }}>✎ ввод…</span>
  if (status.kind === 'saving') return <span style={{ ...base, color: '#1D9E75', background: 'rgba(29,158,117,0.10)' }}>💾 сохранение…</span>
  if (status.kind === 'saved') return <span style={{ ...base, color: '#1D9E75', background: 'rgba(29,158,117,0.10)' }}>✓ сохранено</span>
  return <span style={{ ...base, color: '#b00020', background: 'rgba(176,0,32,0.08)' }}>✗ {status.msg}</span>
}
