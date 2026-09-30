'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { apiFetch } from '@/lib/api'

const STORAGE_KEY = 'mindstrata_cookie_consent_id'
const SHOW_DURATION_MS = 15000          // 15s visible display
const REAPPEAR_DELAY_MS = 2 * 60 * 1000 // then repeat after 2 min
const THANOS_DURATION_MS = 1800         // Thanos disintegration

type Phase = 'hidden' | 'showing' | 'dismissing'

function hasStoredConsent(): boolean {
  if (typeof window === 'undefined') return false
  try {
    return Boolean(localStorage.getItem(STORAGE_KEY))
  } catch {
    return false
  }
}

function makeConsentID(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID()
  }
  return 'c_' + Date.now().toString(36) + '_' + Math.random().toString(36).slice(2, 10)
}

function triggerDust() {
  const animEl = document.getElementById('thanos-displace-anim')
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const begin = (animEl as any)?.beginElement
  if (typeof begin === 'function') {
    try { begin.call(animEl) } catch { /* noop */ }
  }
}

export function CookieBanner() {
  const [phase, setPhase] = useState<Phase>('hidden')

  useEffect(() => {
    // SSR-safe + storage guard. Listeners are always installed: consent may be
    // cleared in another tab or by an admin scenario after mount.
    if (typeof window === 'undefined') return

    let mounted = true
    let showTO = 0
    let dismissTO = 0
    let cycleTO = 0

    const clearAll = () => {
      if (showTO) { clearTimeout(showTO); showTO = 0 }
      if (dismissTO) { clearTimeout(dismissTO); dismissTO = 0 }
      if (cycleTO) { clearTimeout(cycleTO); cycleTO = 0 }
    }

    const doShow = () => {
      // ALL transitions re-check localStorage: it is the single
      // source of truth. After accept, localStorage.setItem is filled ->
      // all branches see consent and render nothing.
      if (!mounted || hasStoredConsent()) return
      setPhase('showing')
      showTO = window.setTimeout(() => {
        if (!mounted || hasStoredConsent()) return
        // 15 s passed without a click -> start Thanos.
        setPhase('dismissing')
        triggerDust()
        dismissTO = window.setTimeout(() => {
          if (!mounted) return
          setPhase('hidden')
          if (!hasStoredConsent()) {
            cycleTO = window.setTimeout(doShow, REAPPEAR_DELAY_MS)
          }
        }, THANOS_DURATION_MS)
      }, SHOW_DURATION_MS)
    }

    if (!hasStoredConsent()) doShow()

    // If consent was cleared on purpose, the banner must come back to life.
    const onConsentClear = () => {
      clearAll()
      if (!hasStoredConsent()) doShow()
    }

    // Sync between tabs: the storage event fires in OTHER tabs
    // of the same origin when localStorage changed.
    const onStorage = (e: StorageEvent) => {
      if (e.key !== STORAGE_KEY) return
      if (e.newValue) {
        // Accept was clicked in another tab -> hide it here.
        clearAll()
        setPhase('hidden')
      } else {
        // Consent was cleared in another tab -> show it here.
        clearAll()
        if (!hasStoredConsent()) doShow()
      }
    }

    window.addEventListener('mindstrata:cookie-consent-cleared', onConsentClear)
    window.addEventListener('storage', onStorage)

    return () => {
      mounted = false
      clearAll()
      window.removeEventListener('mindstrata:cookie-consent-cleared', onConsentClear)
      window.removeEventListener('storage', onStorage)
    }
  }, [])

  if (phase === 'hidden') return null

  const onAccept = async () => {
    let consentID: string
    try {
      consentID = makeConsentID()
      localStorage.setItem(STORAGE_KEY, consentID)
    } catch {
      // private mode / quota full: keep the banner, the user will try again.
      return
    }
    // KEY POINT: after setItem, hasStoredConsent() returns true.
    // All pending tickers inside doShow and its descendants check this
    // on the next tick and just exit silently. So no
    // extra refs are needed.
    setPhase('dismissing')
    triggerDust()
    setTimeout(() => setPhase('hidden'), THANOS_DURATION_MS)
    try {
      await apiFetch('/api/cookie-consent', {
        method: 'POST',
        body: JSON.stringify({
          consentId: consentID,
          sourcePath: typeof window !== 'undefined' ? window.location.pathname : '/',
        }),
      })
    } catch {
      // fire-and-forget: localStorage is already saved, the banner is already gone.
    }
  }

  const thanosing = phase === 'dismissing'

  return (
    <>
      <svg
        aria-hidden
        style={{ position: 'fixed', width: 0, height: 0, pointerEvents: 'none' }}
      >
        <defs>
          <filter id="thanos-displace" x="-20%" y="-20%" width="140%" height="140%">
            <feTurbulence type="fractalNoise" baseFrequency="0.018" numOctaves="2" seed="2" />
            <feDisplacementMap in="SourceGraphic" scale="0">
              <animate
                id="thanos-displace-anim"
                attributeName="scale"
                from="0"
                to="320"
                dur="1.6s"
                begin="indefinite"
                fill="freeze"
              />
            </feDisplacementMap>
          </filter>
        </defs>
      </svg>

      <div
        className="ms-cookie-banner"
        role="region"
        aria-label="Согласие на использование cookie"
        style={{
          position: 'fixed',
          left: 16,
          right: 16,
          bottom: 16,
          zIndex: 1000,
          pointerEvents: 'none',
          filter: thanosing ? 'url(#thanos-displace) blur(1.2px)' : 'none',
          opacity: thanosing ? 0 : 1,
          transform: thanosing ? 'translateY(-10px) scale(1.02)' : 'translateY(0) scale(1)',
          transition: thanosing
            ? 'opacity 1.6s ease-out 0.2s, transform 1.6s ease-out'
            : 'opacity 280ms ease, transform 280ms ease',
        }}
      >
        <div
          className="ms-cookie-card"
          style={{
            maxWidth: 1200,
            margin: '0 auto',
            pointerEvents: 'auto',
            background: 'var(--card)',
            border: '1px solid var(--line)',
            borderRadius: 14,
            boxShadow: '0 24px 50px -28px rgba(13,27,22,0.35)',
            padding: '14px 18px',
            display: 'flex',
            flexDirection: 'row',
            flexWrap: 'wrap',
            alignItems: 'center',
            gap: 14,
            animation: 'ms-cookie-in 280ms ease',
            fontFamily: "'Golos Text', system-ui, sans-serif",
          }}
        >
          <div className="ms-cookie-text" style={{ flex: 1, minWidth: 220, fontSize: 13, lineHeight: 1.5, color: 'var(--foreground)' }}>
            Технические куки нужны для входа. Аналитика — только после «Принять».{' '}
            <Link
              href="/privacy/cookies"
              style={{
                color: 'var(--accent-strong, #1D9E75)',
                textDecoration: 'none',
                borderBottom: '1px dashed var(--accent, #1D9E75)',
                whiteSpace: 'nowrap',
              }}
            >
              Подробнее →
            </Link>
          </div>
          <button
            className="ms-cookie-button"
            type="button"
            onClick={onAccept}
            style={{
              flexShrink: 0,
              padding: '8px 18px',
              borderRadius: 8,
              border: '1px solid var(--accent, #1D9E75)',
              background: 'var(--accent, #1D9E75)',
              color: '#fff',
              fontSize: 13,
              fontWeight: 500,
              cursor: 'pointer',
              fontFamily: "'Golos Text', system-ui, sans-serif",
            }}
          >
            Принять
          </button>
        </div>
        <style>{`
          @keyframes ms-cookie-in { from { transform: translateY(8px); opacity: 0 } to { transform: none; opacity: 1 } }
          @media (max-width: 480px) {
            .ms-cookie-banner {
              left: 10px !important;
              right: 10px !important;
              bottom: 8px !important;
            }
            .ms-cookie-card {
              padding: 10px 12px !important;
              gap: 8px !important;
              border-radius: 12px !important;
            }
            .ms-cookie-text {
              min-width: 0 !important;
              font-size: 12px !important;
              line-height: 1.4 !important;
            }
            .ms-cookie-button {
              padding: 7px 14px !important;
              font-size: 12px !important;
            }
          }
        `}</style>
      </div>
    </>
  )
}
