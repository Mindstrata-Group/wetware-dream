'use client'

import Link from 'next/link'
import { BrandLogo } from '@/components/BrandLogo'
import { useT } from '@/lib/i18n/LocaleProvider'
import { homeText } from './translations'

type TopBarProps = {
  topbarH: number;
  padH: number;
  isFold: boolean;
  isMobile: boolean;
  entryHref: string;
  entryLabel: string;
  profileRole: string | null;
  featureLinks?: Array<{ href: string; label: string }>;
  onLogout: () => void;
}

export function TopBar({
  topbarH,
  padH,
  isFold,
  isMobile,
  entryHref,
  entryLabel,
  profileRole,
  featureLinks = [],
  onLogout,
}: TopBarProps) {
  const t = useT(homeText)
  const buttonStyle = {
    height: isMobile ? 34 : 40,
    padding: isMobile ? '0 11px' : '0 17px',
    borderRadius: 10,
    border: '1px solid var(--line)',
    background: 'rgba(var(--card-rgb), 0.85)',
    color: 'var(--foreground)',
    fontSize: isFold ? 11 : 13,
    fontWeight: 500,
    display: 'inline-flex',
    alignItems: 'center',
    textDecoration: 'none',
    fontFamily: "'Golos Text', system-ui, sans-serif",
    boxShadow: 'none',
    whiteSpace: 'nowrap' as const,
  }

  return (
    <header style={{
      position: 'relative', zIndex: 20,
      height: topbarH, flexShrink: 0,
      display: 'flex', alignItems: 'center',
      justifyContent: 'space-between',
      padding: `0 ${padH}px`,
      background: 'rgba(var(--card-rgb), 0.85)',
      backdropFilter: 'blur(12px)',
      borderBottom: '1px solid var(--line)',
    }}>
      <BrandLogo priority className="ms-brand-link" />

      <nav style={{ display: 'flex', gap: isFold ? 5 : 8, alignItems: 'center', flexShrink: 0 }}>
        {featureLinks.map((link) => (
          <Link key={link.href} href={link.href} style={buttonStyle}>{link.label}</Link>
        ))}
        <Link href={entryHref} style={buttonStyle}>{entryLabel}</Link>

        {profileRole ? (
          <button onClick={onLogout} style={{ ...buttonStyle, cursor: 'pointer' }}>{t.logout}</button>
        ) : (
          <Link href="/register" style={{
            ...buttonStyle,
            border: '1px solid #1D9E75',
            background: 'linear-gradient(160deg, #22b09a 0%, #1D9E75 100%)',
            color: '#fff',
          }}>{isFold ? t.registerShort : t.registerFull}</Link>
        )}
      </nav>
    </header>
  )
}
