'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { BrandLogo } from '@/components/BrandLogo'

import { DEFAULT_LEGAL_DOCS, type LegalDocTile } from '@/lib/legalDocs'
import { getArray, getString } from '@/lib/siteContent'
import { useSiteContent } from '@/lib/useSiteContent'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from './translations'
import { OPERATOR_FOOTER_RU } from '@/lib/operator'

export default function PrivacyHubPage() {
  const [mobile, setMobile] = useState(false)
  const locale = useLocale()
  const t = useT(privacyText)
  const content = useSiteContent()
  const hubTitle = locale === 'en' ? t.hub.title : getString(content, 'privacy.hub.title', 'Документы сервиса')
  const hubSubtitle = locale === 'en' ? t.hub.subtitle : getString(content, 'privacy.hub.subtitle', 'Полный список юридических документов Mindstrata. Согласия — отдельными документами, как требует закон.')
  const docs: LegalDocTile[] = locale === 'en'
    ? [
        { slug: 'terms', title: t.docs.terms.title, hint: t.docs.terms.hint },
        { slug: 'personal-data', title: t.docs.personalData.title, hint: t.docs.personalData.hint },
        { slug: 'consent', title: t.docs.consent.title, hint: t.docs.consent.hint, badge: t.docs.consent.badge },
        { slug: 'cookies', title: t.docs.cookies.title, hint: t.docs.cookies.hint },
        { slug: 'payments', title: t.docs.payments.title, hint: t.docs.payments.hint },
        { slug: 'contacts', title: t.docs.contacts.title, hint: t.docs.contacts.hint },
        { slug: 'withdraw', title: t.docs.withdraw.title, hint: t.docs.withdraw.hint },
      ]
    : getArray<LegalDocTile>(content, 'privacy.docs', DEFAULT_LEGAL_DOCS)
  const footerCopyright = getString(content, 'landing.footer.copyright', OPERATOR_FOOTER_RU)
  useEffect(() => {
    const check = () => setMobile(window.innerWidth < 900)
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])

  return (
    <div style={{ minHeight: '100vh', background: 'var(--background)', fontFamily: '"Golos Text", system-ui, sans-serif', color: 'var(--foreground)' }}>
      <header style={{ position: 'sticky', top: 0, zIndex: 20, height: 64, display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 32px', background: 'color-mix(in oklab, var(--card) 88%, transparent)', backdropFilter: 'blur(12px)', borderBottom: '1px solid var(--line)' }}>
        <BrandLogo priority />
        <div style={{ display: 'flex', gap: 8 }}>
          <Link href="/" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'color-mix(in oklab, var(--card) 88%, transparent)', color: 'var(--foreground)', fontSize: 13, fontWeight: 500, textDecoration: 'none' }}>{t.hub.navHome}</Link>
          <Link href="/register" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'color-mix(in oklab, var(--card) 88%, transparent)', color: 'var(--foreground)', fontSize: 13, textDecoration: 'none' }}>{t.hub.navRegister}</Link>
        </div>
      </header>

      <div style={{ maxWidth: 1100, margin: '0 auto', padding: '48px 32px' }}>
        <div style={{ marginBottom: 40 }}>
          <h1 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontWeight: 600, fontSize: mobile ? 26 : 36, letterSpacing: '-0.02em', margin: '0 0 12px', color: 'var(--foreground)' }}>
            {hubTitle}
          </h1>
          <p style={{ fontSize: 15, color: 'var(--muted)', margin: 0, maxWidth: 680 }}>
            {hubSubtitle}
          </p>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: mobile ? '1fr' : 'repeat(2, 1fr)', gap: 16 }}>
          {docs.map(doc => (
            <Link
              key={doc.slug}
              href={`/privacy/${doc.slug}`}
              style={{
                display: 'block',
                padding: '20px 22px',
                background: 'var(--card)',
                border: '1px solid var(--line)',
                borderRadius: 14,
                textDecoration: 'none',
                color: 'inherit',
                transition: 'transform 0.15s, border-color 0.15s, box-shadow 0.2s',
              }}
              onMouseEnter={e => {
                e.currentTarget.style.borderColor = 'var(--accent)'
                e.currentTarget.style.transform = 'translateY(-1px)'
              }}
              onMouseLeave={e => {
                e.currentTarget.style.borderColor = 'var(--line)'
                e.currentTarget.style.transform = 'translateY(0)'
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 8 }}>
                <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 16, fontWeight: 600, margin: 0, lineHeight: 1.3, color: 'var(--foreground)' }}>
                  {doc.title}
                </h2>
                {doc.badge && (
                  <span style={{ fontSize: 11, padding: '2px 8px', borderRadius: 6, background: 'color-mix(in oklab, var(--accent) 14%, var(--card))', color: 'var(--accent-strong)', border: '1px solid var(--accent)', whiteSpace: 'nowrap', flexShrink: 0 }}>
                    {doc.badge}
                  </span>
                )}
              </div>
              <p style={{ fontSize: 13, color: 'var(--muted)', margin: 0, lineHeight: 1.5 }}>
                {doc.hint}
              </p>
              <div style={{ marginTop: 12, fontSize: 12, color: 'var(--accent-strong)', fontWeight: 500 }}>
                {t.hub.openLabel}
              </div>
            </Link>
          ))}
        </div>

        <div style={{ marginTop: 64, paddingTop: 32, borderTop: '1px solid var(--line)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
          <span style={{ fontSize: 13, color: 'var(--muted)' }}>{footerCopyright}</span>
          <Link href="/" style={{ fontSize: 13, color: 'var(--accent-strong)', fontWeight: 500, textDecoration: 'none' }}>{t.hub.footerHome}</Link>
        </div>
      </div>
    </div>
  )
}
