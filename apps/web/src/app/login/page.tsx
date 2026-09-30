'use client'

import { Suspense, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { BrandLogo } from '@/components/BrandLogo'
import { roleHome } from '@/lib/api'
import { useApi } from '@/lib/useApi'
import { useT } from '@/lib/i18n/LocaleProvider'
import { authText } from '../_auth/translations'

const green = '#1D9E75'
const greenDark = '#0F6E56'
const ink = '#0D1B16'
const ink50 = '#7A9A8E'
const ink10 = 'rgba(0,0,0,0.07)'
const bg = 'var(--background)'

function YandexLogo() {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24">
      <circle cx="12" cy="12" r="12" fill="#FC3F1D" />
      <path d="M13.4 17.1V6.9h-1.55c-1.95 0-3.05 1.05-3.05 2.5 0 1.18.55 1.95 1.65 2.6L8.6 17.1h1.7l1.55-4.45-.7-.45c-1.05-.7-1.55-1.3-1.55-2.3 0-.85.55-1.5 1.85-1.5h.35V17.1h1.6z" fill="#fff" />
    </svg>
  )
}

function LoginContent() {
  const router = useRouter()
  const params = useSearchParams()
  const t = useT(authText)
  const { data: session } = useApi<{ user?: { role: string } }>('/api/auth/me')
  const nextParam = params?.get('next') || ''
  const next = nextParam.startsWith('/') ? nextParam : '/profile'
  const errorCode = params?.get('error') || ''
  const errorText = errorCode ? (t.oauthErrors[errorCode as keyof typeof t.oauthErrors] || t.oauthErrors.default) : ''
  const retryHref = errorCode === 'oauth_account'
    ? `/register?next=${encodeURIComponent(next)}`
    : `/api/auth/oauth/yandex/start?next=${encodeURIComponent(next)}&force_account=1`
  const retryLabel = errorCode === 'oauth_account' ? t.login.registerAccount : t.login.retryAccount

  useEffect(() => {
    if (session?.user) router.replace(roleHome(session.user.role))
  }, [router, session])

  return (
    <div style={{ minHeight: '100vh', background: bg, fontFamily: '"Golos Text", system-ui, sans-serif', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <div aria-hidden style={{ position: 'absolute', inset: 0, opacity: 0.4, backgroundImage: 'radial-gradient(circle, var(--foreground) 1px, transparent 1px)', backgroundSize: '24px 24px', pointerEvents: 'none' }} />
      <header style={{ position: 'sticky' as const, top: 0, zIndex: 20, height: 64, display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 clamp(14px, 4%, 32px)', background: 'rgba(var(--card-rgb), 0.9)', backdropFilter: 'blur(12px)', borderBottom: '1px solid var(--line)' }}>
        <BrandLogo priority />
        <div style={{ display: 'flex', gap: 8 }}>
          <Link href="/" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'rgba(var(--card-rgb), 0.84)', color: 'var(--foreground)', fontSize: 13, fontWeight: 500, textDecoration: 'none', whiteSpace: 'nowrap' }}>{t.nav.home}</Link>
          <Link href="/register" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'rgba(var(--card-rgb), 0.84)', color: 'var(--foreground)', fontSize: 13, textDecoration: 'none', whiteSpace: 'nowrap' }}>{t.nav.register}</Link>
        </div>
      </header>

      <main style={{ position: 'relative', flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '40px 20px' }}>
        <div style={{ width: '100%', maxWidth: 400, background: 'var(--card)', border: '1px solid var(--line)', borderRadius: 20, padding: '40px 36px', boxShadow: '0 20px 60px -20px rgba(13,27,22,0.15)' }}>
          <h1 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontWeight: 600, fontSize: 28, letterSpacing: '-0.02em', margin: '0 0 8px', color: 'var(--foreground)' }}>{t.login.title}</h1>
          <p style={{ fontSize: 14, color: 'var(--muted)', margin: '0 0 32px', lineHeight: 1.5 }}>{t.login.subtitle}</p>

          <a href={`/api/auth/oauth/yandex/start?next=${encodeURIComponent(next)}`} style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 10, width: '100%', height: 52, borderRadius: 12, background: '#000', color: '#fff', fontSize: 15, fontWeight: 500, textDecoration: 'none', fontFamily: 'inherit' }}>
            <YandexLogo />
            {t.login.yandexButton}
          </a>

          {errorText && (
            <div style={{ marginTop: 16, padding: '12px 14px', borderRadius: 10, background: '#FBF0F0', border: '1px solid #EFB9B9', color: '#8a3333', fontSize: 13, lineHeight: 1.5 }}>
              {errorText}
              <div style={{ marginTop: 8 }}>
                <a href={retryHref} style={{ color: '#C2453E', fontWeight: 500, fontSize: 13 }}>
                  {retryLabel}
                </a>
              </div>
            </div>
          )}

          <div style={{ marginTop: 28, paddingTop: 24, borderTop: '1px solid var(--line)', display: 'flex', justifyContent: 'center', gap: 20 }}>
            <Link href="/register" style={{ fontSize: 13, color: greenDark, fontWeight: 500, textDecoration: 'none' }}>{t.login.registerLink}</Link>
            <Link href="/access?force=promo" style={{ fontSize: 13, color: 'var(--muted)', textDecoration: 'none' }}>{t.login.promoLink}</Link>
          </div>
        </div>
      </main>
    </div>
  )
}

export default function LoginPage() {
  const t = useT(authText)
  return (
    <Suspense fallback={<div style={{ minHeight: '100vh', background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center', color: ink50, fontFamily: '"Golos Text",system-ui,sans-serif' }}>{t.login.loading}</div>}>
      <LoginContent />
    </Suspense>
  )
}
