'use client'

import { useState } from 'react'
import type { CheckItem } from './_types'
import { StatusIcon } from './StatusIcon'

function CheckRow({ check }: { check: CheckItem }) {
  const [open, setOpen] = useState(false)
  const hasDetails = !!(check.detail || check.json != null)
  return (
    <div style={{ borderBottom: '1px solid rgba(0,0,0,0.05)', padding: '10px 0' }}>
      <div
        style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: hasDetails ? 'pointer' : 'default' }}
        onClick={() => hasDetails && setOpen(v => !v)}
      >
        <StatusIcon status={check.status} />
        <span style={{ flex: 1, fontSize: 14, color: 'var(--foreground)' }}>{check.name}</span>
        {check.detail && (
          <span style={{ fontSize: 12, color: 'var(--muted)', maxWidth: 240, textAlign: 'right', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {check.detail}
          </span>
        )}
        {hasDetails && (
          <svg width={12} height={12} viewBox="0 0 12 12" fill="none" stroke="var(--muted)" strokeWidth="1.5" strokeLinecap="round"
            style={{ transform: open ? 'rotate(180deg)' : undefined, transition: 'transform 0.15s', flexShrink: 0 }}>
            <path d="M2 4l4 4 4-4" />
          </svg>
        )}
      </div>
      {open && (
        <div style={{ paddingLeft: 32, paddingTop: 8 }}>
          {check.detail && <div style={{ fontSize: 13, color: 'var(--foreground)', lineHeight: 1.5 }}>{check.detail}</div>}
          {check.json != null && (
            <pre style={{ margin: '8px 0 0', padding: '10px 12px', borderRadius: 8, background: 'var(--foreground)', color: '#E1F5EE', fontSize: 12, lineHeight: 1.5, overflowX: 'auto', maxHeight: 200, scrollbarWidth: 'none' }}>
              {JSON.stringify(check.json, null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  )
}

export { CheckRow }
