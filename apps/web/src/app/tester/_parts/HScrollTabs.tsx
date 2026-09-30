'use client'

import type { CheckGroup } from './_types'

function HScrollTabs({ groups, active, onSelect }: { groups: CheckGroup[]; active: string; onSelect: (id: string) => void }) {
  return (
    <div style={{ display: 'flex', gap: 6, overflowX: 'auto', scrollbarWidth: 'none', paddingBottom: 2 }}>
      {groups.map(g => {
        const errs = g.checks.filter(c => c.status === 'fail').length
        const warns = g.checks.filter(c => c.status === 'warn').length
        return (
          <button key={g.id} onClick={() => onSelect(g.id)} style={{
            flexShrink: 0, height: 34, padding: '0 14px', borderRadius: 999,
            border: `1px solid ${g.id === active ? '#1D9E75' : 'var(--line)'}`,
            background: g.id === active ? '#E1F5EE' : 'var(--card)',
            color: g.id === active ? '#0F6E56' : 'var(--foreground)',
            fontSize: 13, fontWeight: 500, cursor: 'pointer', fontFamily: 'inherit',
            display: 'flex', alignItems: 'center', gap: 6,
          }}>
            {g.label}
            {errs > 0 && <span style={{ fontSize: 11, fontWeight: 700, color: '#C2453E' }}>{errs}</span>}
            {errs === 0 && warns > 0 && <span style={{ fontSize: 11, fontWeight: 700, color: '#B07A1A' }}>{warns}</span>}
          </button>
        )
      })}
    </div>
  )
}

export { HScrollTabs }
