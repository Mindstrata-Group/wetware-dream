'use client'

import Link from 'next/link'
import { useT } from '@/lib/i18n/LocaleProvider'
import { aboutText } from '../translations'

export function CtaSection({ ctaTitleF, ctaSubtitleF, isMobile }: {
  ctaTitleF: string;
  ctaSubtitleF: string;
  isMobile: boolean;
}) {
  const t = useT(aboutText)
  return (
    <>
          <section className="ms-about-section" style={{
            padding: isMobile ? '28px 20px' : '36px 40px',
            borderRadius: 20,
            background: 'linear-gradient(135deg, #0F6E56 0%, #1D9E75 100%)',
            display: 'flex',
            flexDirection: isMobile ? 'column' : 'row',
            alignItems: isMobile ? 'flex-start' : 'center',
            justifyContent: 'space-between',
            gap: 20,
          }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <div style={{
                fontSize: isMobile ? 20 : 26,
                fontWeight: 700,
                color: '#fff',
                fontFamily: "'Unbounded', system-ui, sans-serif",
                lineHeight: 1.1,
                letterSpacing: '-0.02em',
              }}>{ctaTitleF}</div>
              <div style={{ fontSize: 14, color: 'rgba(255,255,255,0.75)', lineHeight: 1.45 }}>
                {ctaSubtitleF}
              </div>
            </div>
            <div style={{ display: 'flex', gap: 10, flexShrink: 0, flexWrap: 'wrap' }}>
              <Link href="/access?force=promo" style={{
                height: 46, padding: '0 24px', borderRadius: 11,
                background: '#fff',
                // The #fff background is fixed (inside the green gradient of the CTA section),
                // so the text colour is fixed dark as well; it does not depend on the theme.
                // Otherwise in dark mode var(--foreground) = white -> white-on-white.
                color: '#0F6E56',
                fontSize: 14, fontWeight: 600,
                display: 'inline-flex', alignItems: 'center',
                textDecoration: 'none',
                fontFamily: "'Golos Text', system-ui, sans-serif",
                boxShadow: 'none',
                whiteSpace: 'nowrap',
              }}>{t.ctaButtons.promo}</Link>
              <Link href="/login" style={{
                height: 46, padding: '0 24px', borderRadius: 11,
                background: 'rgba(255,255,255,0.15)',
                border: '1px solid rgba(255,255,255,0.3)',
                color: '#fff',
                fontSize: 14, fontWeight: 500,
                display: 'inline-flex', alignItems: 'center',
                textDecoration: 'none',
                fontFamily: "'Golos Text', system-ui, sans-serif",
                boxShadow: 'none',
                whiteSpace: 'nowrap',
              }}>{t.ctaButtons.login}</Link>
            </div>
          </section>

    </>
  );
}
