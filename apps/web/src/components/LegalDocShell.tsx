'use client'

import { ReactNode, useEffect, useState } from 'react'
import Link from 'next/link'
import { BrandLogo } from '@/components/BrandLogo'
import { EditableLegalBody } from '@/components/EditableLegalBody'
import { getString } from '@/lib/siteContent'
import { useSiteContent } from '@/lib/useSiteContent'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from '@/app/privacy/translations'
import { OPERATOR_FOOTER_RU } from '@/lib/operator'

export interface LegalDocShellProps {
  title: string
  updated?: string
  subtitle?: string
  children: ReactNode
  contentKey?: string
}

export function LegalDocShell({ title, updated, subtitle, children, contentKey }: LegalDocShellProps) {
  const [mobile, setMobile] = useState(false)
  const locale = useLocale()
  const t = useT(privacyText)
  const content = useSiteContent()
  const isRu = locale === 'ru'
  const resolvedTitle = isRu && contentKey ? getString(content, `${contentKey}.title`, title) : title
  const resolvedSubtitle = isRu && contentKey ? getString(content, `${contentKey}.subtitle`, subtitle ?? '') : (subtitle ?? '')
  const resolvedBody = isRu && contentKey ? getString(content, `${contentKey}.body`, '') : ''
  const updatedLabel = updated ?? t.updated
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
          <Link href="/privacy" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'color-mix(in oklab, var(--card) 88%, transparent)', color: 'var(--foreground)', fontSize: 13, fontWeight: 500, textDecoration: 'none' }}>{t.shell.backToDocs}</Link>
          <Link href="/" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'color-mix(in oklab, var(--card) 88%, transparent)', color: 'var(--foreground)', fontSize: 13, textDecoration: 'none' }}>{t.shell.home}</Link>
        </div>
      </header>

      <div style={{ maxWidth: 820, margin: '0 auto', padding: '48px 32px' }}>
        <article>
          <div style={{ marginBottom: 32 }}>
            <h1 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontWeight: 600, fontSize: mobile ? 22 : 30, letterSpacing: '-0.02em', margin: '0 0 12px', color: 'var(--foreground)' }}>
              {resolvedTitle}
            </h1>
            <p style={{ fontSize: 14, color: 'var(--muted)', margin: 0 }}>
              {resolvedSubtitle ? resolvedSubtitle + ' · ' : t.shell.brandPrefix}{t.shell.lastUpdated}{updatedLabel}
            </p>
          </div>

          <div style={{ fontSize: 15, lineHeight: 1.75, color: 'var(--foreground)' }}>
            {resolvedBody ? <EditableLegalBody body={resolvedBody} /> : children}
          </div>

          <div style={{ marginTop: 64, paddingTop: 32, borderTop: '1px solid var(--line)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
            <span style={{ fontSize: 13, color: 'var(--muted)' }}>{footerCopyright}</span>
            <Link href="/privacy" style={{ fontSize: 13, color: 'var(--accent-strong)', fontWeight: 500, textDecoration: 'none' }}>{t.shell.footerBackToDocs}</Link>
          </div>
        </article>
      </div>
    </div>
  )
}
