'use client'

import { useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { BrandLogo } from '@/components/BrandLogo'
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

import { redirectToRegisterOauth } from './_parts/oauth'

export default function RegisterPage() {
  const t = useT(authText)
  const [agreed, setAgreed] = useState(false)
  const [shakeAgree, setShakeAgree] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const shakeTimers = useRef<number[]>([])

  const clearShakeTimers = () => {
    for (const timer of shakeTimers.current) {
      window.clearTimeout(timer)
    }
    shakeTimers.current = []
  }

  useEffect(() => clearShakeTimers, [])

  const toggleAgreed = () => {
    setAgreed(v => {
      const next = !v
      if (next) setError(null)
      return next
    })
  }

  const handleYandexClick = () => {
    if (!agreed) {
      clearShakeTimers()
      setShakeAgree(false)
      shakeTimers.current = [
        window.setTimeout(() => setShakeAgree(true), 0),
        window.setTimeout(() => setShakeAgree(false), 650),
      ]
      setError(t.register.agreeError)
      return
    }
    const next = new URLSearchParams(window.location.search).get('next')
    redirectToRegisterOauth(next)
  }

  return (
    <div style={{ minHeight: '100vh', background: bg, fontFamily: '"Golos Text", system-ui, sans-serif', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <div aria-hidden style={{ position: 'absolute', inset: 0, opacity: 0.4, backgroundImage: 'radial-gradient(circle, var(--foreground) 1px, transparent 1px)', backgroundSize: '24px 24px', pointerEvents: 'none' }} />
      {/* Header — consistent with login/landing, theme-aware */}
      <header style={{ position: 'sticky' as const, top: 0, zIndex: 20, height: 64, display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 clamp(14px, 4%, 32px)', background: 'rgba(var(--card-rgb), 0.88)', backdropFilter: 'blur(12px)', borderBottom: '1px solid var(--line)' }}>
        <BrandLogo priority />
        <div style={{ display: 'flex', gap: 8 }}>
          <Link href="/" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'rgba(var(--card-rgb), 0.88)', color: 'var(--foreground)', fontSize: 13, fontWeight: 500, textDecoration: 'none', whiteSpace: 'nowrap' }}>{t.nav.home}</Link>
          <Link href="/login" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'rgba(var(--card-rgb), 0.88)', color: 'var(--foreground)', fontSize: 13, textDecoration: 'none', whiteSpace: 'nowrap' }}>{t.nav.login}</Link>
        </div>
      </header>

      <style>{`@keyframes ms-register-shake { 0%, 100% { transform: translateX(0) } 15%, 45%, 75% { transform: translateX(-6px) } 30%, 60%, 90% { transform: translateX(6px) } }`}</style>
      <main style={{ position: 'relative', flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '40px 20px' }}>
        <div style={{ width: '100%', maxWidth: 400, background: 'var(--card)', border: '1px solid var(--line)', borderRadius: 20, padding: '40px 36px', boxShadow: '0 20px 60px -20px rgba(13,27,22,0.15)' }}>
          <h1 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontWeight: 600, fontSize: 28, letterSpacing: '-0.02em', margin: '0 0 8px', color: 'var(--foreground)' }}>{t.register.title}</h1>
          <p style={{ fontSize: 14, color: 'var(--muted)', margin: '0 0 28px', lineHeight: 1.5 }}>
            {t.register.subtitle}
          </p>

          {/* Yandex button — redirects only after explicit agreement checkbox. */}
          <button
            onClick={handleYandexClick}
            style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 10, width: '100%', height: 52, borderRadius: 12, background: '#000', color: '#fff', fontSize: 15, fontWeight: 500, fontFamily: 'inherit', cursor: 'pointer', border: 'none' }}
          >
            <YandexLogo />
            {t.register.yandexButton}
          </button>

          {/* TOS checkbox — required before OAuth redirect. */}
          <div
            role="checkbox"
            aria-checked={agreed}
            tabIndex={0}
            onClick={toggleAgreed}
            onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleAgreed() } }}
            style={{ display: 'flex', alignItems: 'flex-start', gap: 10, margin: '20px 0', cursor: 'pointer', animation: shakeAgree ? 'ms-register-shake 0.45s ease' : 'none' }}
          >
            <div style={{ width: 18, height: 18, borderRadius: 5, border: `2px solid ${agreed ? green : 'var(--line)'}`, background: agreed ? green : 'var(--card)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, marginTop: 1, transition: 'all 0.15s', cursor: 'pointer' }}>
              {agreed && <svg width={10} height={8} viewBox="0 0 10 8" fill="none"><path d="M1 4l3 3 5-6" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>}
            </div>
            <span style={{ fontSize: 13, color: 'var(--muted)', lineHeight: 1.5 }}>
              {t.register.agreePrefix}{' '}
              <Link href="/privacy" style={{ color: greenDark, textDecoration: 'none', borderBottom: '1px dashed rgba(29,158,117,0.4)' }} onClick={e => e.stopPropagation()}>
                {t.register.privacyLink}
              </Link>{' '}
              {t.register.agreeSuffix}
            </span>
          </div>
          {error && (
            <div role="alert" style={{ fontSize: 13, color: '#c0392b', marginTop: -10, marginBottom: 12 }}>
              {error}
            </div>
          )}

          <Link href="/access?force=promo" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: '100%', height: 44, borderRadius: 12, background: 'transparent', color: greenDark, fontSize: 14, fontWeight: 500, textDecoration: 'none', fontFamily: 'inherit', cursor: 'pointer', border: '1px solid rgba(29,158,117,0.35)', marginTop: 10 }}>
            {t.register.promoLink}
          </Link>

          <div style={{ marginTop: 24, paddingTop: 20, borderTop: '1px solid var(--line)', display: 'flex', justifyContent: 'center' }}>
            <span style={{ fontSize: 13, color: 'var(--muted)' }}>
              {t.register.haveAccount}{' '}
              <Link href="/login" style={{ color: greenDark, fontWeight: 500, textDecoration: 'none' }}>{t.register.loginLink}</Link>
            </span>
          </div>
        </div>
      </main>
    </div>
  )
}
