'use client'

import Link from 'next/link'
import { CtaLink } from './CtaLink'

type MobileCtaProps = {
  isFold: boolean;
  ctaPrimaryLabel: string;
  legalLinkLabel: string;
  footerCopyright: string;
  featureLinks?: Array<{ href: string; label: string }>;
}

export function MobileCta({ isFold, ctaPrimaryLabel, legalLinkLabel, footerCopyright, featureLinks = [] }: MobileCtaProps) {
  return (
    <div style={{
      position: 'relative', zIndex: 2, flexShrink: 0,
      marginBottom: featureLinks.length > 0 ? 68 : 0,
      padding: isFold ? '7px 14px 10px' : '9px 16px 14px',
      display: 'flex', flexDirection: 'column', gap: 7,
      background: 'color-mix(in oklab, var(--background) 88%, transparent)',
      backdropFilter: 'blur(8px)',
      borderTop: '1px solid var(--line)',
    }}>
      <CtaLink href="/access?force=promo" primary fullWidth>{ctaPrimaryLabel}</CtaLink>
      {featureLinks.length > 0 && (
        <div style={{ display: 'grid', gridTemplateColumns: featureLinks.length > 1 ? '1fr 1fr' : '1fr', gap: 7 }}>
          {featureLinks.map((link) => (
            <CtaLink key={link.href} href={link.href} fullWidth small>{link.label}</CtaLink>
          ))}
        </div>
      )}
      <div style={{ textAlign: 'center' }}>
        <Link href="/privacy" style={{
          fontSize: 11, color: 'var(--muted)', textDecoration: 'none',
          borderBottom: '1px dashed rgba(0,0,0,0.12)',
          fontFamily: "'Golos Text', system-ui, sans-serif",
        }}>{legalLinkLabel}</Link>
      </div>
      <div style={{
        textAlign: 'center', fontSize: 10, color: 'var(--muted)',
        fontFamily: "'Golos Text', system-ui, sans-serif", opacity: 0.7,
      }}>
        {footerCopyright}
      </div>
    </div>
  )
}
