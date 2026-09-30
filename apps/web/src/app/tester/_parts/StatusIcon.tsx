'use client'

import type { CheckStatus } from './_types'

function StatusIcon({ status }: { status: CheckStatus }) {
  const map: Record<CheckStatus, { icon: string; color: string; bg: string }> = {
    ok:      { icon: '✓', color: '#1D9E75', bg: '#E1F5EE' },
    warn:    { icon: '!', color: '#B07A1A', bg: '#FEF3CD' },
    fail:    { icon: '✕', color: '#C2453E', bg: '#FBF0F0' },
    skip:    { icon: '—', color: 'var(--muted)', bg: 'var(--background)' },
    running: { icon: '…', color: '#2F6EAA', bg: '#EAF2FB' },
  }
  const { icon, color, bg } = map[status]
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 22, height: 22, borderRadius: 6, background: bg, color, fontSize: 11, fontWeight: 700, flexShrink: 0 }}>
      {icon}
    </span>
  )
}

export { StatusIcon }
