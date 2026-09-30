'use client'

import { useEffect, useRef } from 'react'
import { MS } from './_theme'

function DotsBackground({ opacity = 0.4 }: { opacity?: number }) {
  return (
    <div
      aria-hidden
      style={{
        position: 'absolute',
        inset: 0,
        opacity,
        backgroundImage:
          'radial-gradient(circle, var(--foreground) 1px, transparent 1px)',
        backgroundSize: '24px 24px',
        pointerEvents: 'none',
      }}
    />
  )
}

export { DotsBackground }
