'use client'

import Link from 'next/link'

function CtaLink({ href, children, primary, fullWidth, small }: {
  href: string
  children: React.ReactNode
  primary?: boolean
  fullWidth?: boolean
  small?: boolean
}) {
  return (
    <Link
      href={href}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        height: small ? 38 : 46,
        padding: small ? '0 18px' : '0 26px',
        borderRadius: 11,
        border: primary ? '1px solid #1D9E75' : '1px solid var(--line)',
        background: primary
          ? 'linear-gradient(160deg, #22b09a 0%, #1D9E75 100%)'
          : 'color-mix(in oklab, var(--card) 84%, transparent)',
        color: primary ? '#fff' : 'var(--foreground)',
        fontSize: small ? 13 : 14,
        fontWeight: 500,
        textDecoration: 'none',
        fontFamily: "'Golos Text', system-ui, sans-serif",
        cursor: 'pointer',
        whiteSpace: 'nowrap',
        boxShadow: 'none',
        width: fullWidth ? '100%' : undefined,
        WebkitTapHighlightColor: 'transparent',
        transition: 'opacity 0.15s',
      }}
    >
      {children}
    </Link>
  )
}

export { CtaLink }
