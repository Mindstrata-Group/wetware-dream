'use client'

import { useEffect, useRef } from 'react'

function ModeStrip({
  demos, active, onPick, compact,
}: {
  demos: { name: string }[]
  active: number
  onPick: (i: number) => void
  compact: boolean
}) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = ref.current?.querySelector<HTMLElement>('[data-active="true"]')
    el?.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' })
  }, [active])

  return (
    <div style={{ position: 'relative' }}>
      <div
        ref={ref}
        style={{
          display: 'flex', gap: 8, overflowX: 'auto', scrollbarWidth: 'none',
          WebkitOverflowScrolling: 'touch', paddingBottom: 4, paddingRight: 28,
        }}
      >
        {demos.map((d, i) => (
          <button
            key={i}
            data-active={i === active ? 'true' : undefined}
            onClick={() => onPick(i)}
            style={{
              flexShrink: 0,
              height: compact ? 30 : 34,
              padding: '0 13px',
              borderRadius: 999,
              border: `1px solid ${i === active ? '#1D9E75' : 'var(--line)'}`,
              background: i === active ? 'color-mix(in oklab, var(--accent) 14%, var(--card))' : 'color-mix(in oklab, var(--card) 80%, transparent)',
              color: i === active ? 'var(--accent-strong)' : 'var(--muted)',
              fontSize: compact ? 12 : 13,
              fontWeight: 500,
              cursor: 'pointer',
              whiteSpace: 'nowrap',
              fontFamily: "'Golos Text', system-ui, sans-serif",
              transition: 'all 0.15s',
              boxShadow: 'none',
              outline: 'none',
            }}
          >
            {d.name}
          </button>
        ))}
      </div>
      <div style={{
        position: 'absolute', right: 0, top: 0, bottom: 0, width: 32,
        pointerEvents: 'none',
        background: 'linear-gradient(to right, transparent, var(--background))',
      }} />
    </div>
  )
}

export { ModeStrip }
