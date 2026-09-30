'use client'

import Link from 'next/link'
import type { Conversation } from './types'
import { CtaLink } from './CtaLink'
import { ModeStrip } from './ModeStrip'

type HeroSectionProps = {
  heroTitle: string;
  heroSubtitle: string;
  ctaPrimaryLabel: string;
  legalLinkLabel: string;
  isFold: boolean;
  isMobile: boolean;
  isDesktop: boolean;
  h1Size: number;
  stripDemos: Conversation[];
  activeStrip: number;
  onStripPick: (index: number) => void;
  featureLinks?: Array<{ href: string; label: string }>;
}

export function HeroSection({
  heroTitle,
  heroSubtitle,
  ctaPrimaryLabel,
  legalLinkLabel,
  isFold,
  isMobile,
  isDesktop,
  h1Size,
  stripDemos,
  activeStrip,
  onStripPick,
  featureLinks = [],
}: HeroSectionProps) {
  return (
    <section style={{
      display: 'flex', flexDirection: 'column',
      gap: isFold ? 7 : isMobile ? 9 : 18,
      minWidth: 0, flexShrink: 0,
      width: isDesktop ? 'clamp(320px,38%,480px)' : '100%',
    }}>
      <h1 style={{
        fontFamily: "'Unbounded', system-ui, sans-serif",
        fontWeight: 600,
        fontSize: h1Size,
        lineHeight: 1.1,
        letterSpacing: 0,
        margin: 0,
        color: 'var(--foreground)',
        wordBreak: 'normal',
        overflowWrap: 'break-word',
      }}>
        {heroTitle}
      </h1>

      {!isFold && (
        <p style={{
          fontSize: isMobile ? 13 : 16,
          color: 'var(--muted)',
          lineHeight: 1.45,
          margin: 0,
        }}>
          {heroSubtitle}
        </p>
      )}

      <ModeStrip
        demos={stripDemos}
        active={activeStrip >= 0 ? activeStrip : 0}
        onPick={onStripPick}
        compact={isMobile}
      />

      {isDesktop && (
        <div style={{ display: 'flex', gap: 12, alignItems: 'center', marginTop: 2, flexWrap: 'wrap' }}>
          <CtaLink href="/access?force=promo" primary>{ctaPrimaryLabel}</CtaLink>
          {featureLinks.map((link) => (
            <CtaLink key={link.href} href={link.href}>{link.label}</CtaLink>
          ))}
          <Link href="/privacy" style={{
            color: 'var(--muted)', fontSize: 12, textDecoration: 'none',
            borderBottom: '1px dashed rgba(0,0,0,0.15)',
            fontFamily: "'Golos Text', system-ui, sans-serif",
          }}>{legalLinkLabel}</Link>
        </div>
      )}
    </section>
  )
}
