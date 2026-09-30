'use client'

import Link from 'next/link'
import { MS, FONT_HEAD, FONT_MONO } from '../_parts/_theme'
import { useT } from '@/lib/i18n/LocaleProvider'
import { accessText } from '../translations'

export function InputView({ helpPrefix, testAccessUrl, testAccessLinkLabel, shopLink, code, setCode, error, setError, busy, submit }: any) {
  const t = useT(accessText)
  return (
    <>
      <main
        style={{
          position: 'relative',
          flex: 1,
          padding: '40px 20px',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
        }}
      >
        <div
          style={{
            width: '100%',
            maxWidth: 440,
            background: MS.surface,
            border: `1px solid ${MS.ink10}`,
            borderRadius: 20,
            padding: '40px 36px',
            boxShadow: '0 20px 60px -20px rgba(13,27,22,0.15)',
          }}
        >
          <h1
            style={{
              fontFamily: FONT_HEAD,
              fontWeight: 600,
              fontSize: 26,
              letterSpacing: '-0.02em',
              margin: '0 0 8px',
              color: MS.ink,
            }}
          >
            {t.input.title}
          </h1>
          <p
            style={{
              fontSize: 14,
              color: MS.ink70,
              margin: '0 0 28px',
              lineHeight: 1.5,
            }}
          >
            {helpPrefix}{' '}
            <a
              href={testAccessUrl}
              target="_blank"
              rel="noreferrer"
              style={{
                color: MS.greenDark,
                fontWeight: 500,
                textDecoration: 'none',
                borderBottom: `1px solid ${MS.green}`,
              }}
            >
              {testAccessLinkLabel}
            </a>
            .
          </p>

          <form
            onSubmit={submit}
            style={{ display: 'flex', flexDirection: 'column', gap: 14 }}
          >
            <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              <span
                style={{ fontSize: 12, color: MS.ink70, fontWeight: 500 }}
              >
                {t.input.promoLabel}
              </span>
              <input
                value={code}
                onChange={e => {
                  setCode(e.target.value.toUpperCase())
                  if (error) setError('')
                }}
                placeholder={t.input.placeholder}
                autoFocus
                maxLength={64}
                disabled={busy}
                style={{
                  width: '100%',
                  height: 56,
                  borderRadius: 12,
                  border: `1.5px solid ${error ? MS.red : MS.ink20}`,
                  padding: '0 16px',
                  fontSize: 16,
                  fontFamily: FONT_MONO,
                  fontWeight: 600,
                  letterSpacing: '1px',
                  color: MS.ink,
                  background: MS.surface,
                  outline: 'none',
                  boxSizing: 'border-box',
                  textAlign: 'center',
                  textTransform: 'uppercase',
                  transition: 'border-color 0.15s',
                }}
              />
            </label>

            {error && (
              <div
                style={{
                  fontSize: 13,
                  color: MS.red,
                  background: 'rgba(194,69,62,0.08)',
                  border: '1px solid rgba(194,69,62,0.2)',
                  borderRadius: 8,
                  padding: '10px 12px',
                  lineHeight: 1.4,
                }}
              >
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={busy}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: '100%',
                height: 52,
                borderRadius: 12,
                background: busy ? MS.ink50 : MS.green,
                color: '#fff',
                fontSize: 15,
                fontWeight: 600,
                border: 'none',
                cursor: busy ? 'not-allowed' : 'pointer',
                fontFamily: 'inherit',
                transition: 'background 0.15s',
              }}
            >
              {busy ? t.input.submitBusy : t.input.submitIdle}
            </button>
          </form>

          {shopLink && (
            <Link
              href={shopLink.href}
              style={{
                marginTop: 14,
                minHeight: 44,
                borderRadius: 12,
                border: `1px solid ${MS.green}`,
                background: MS.greenLight,
                color: MS.greenDark,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 14,
                fontWeight: 600,
                textDecoration: 'none',
              }}
            >
              {shopLink.label}
            </Link>
          )}

          <div
            style={{
              marginTop: 24,
              paddingTop: 20,
              borderTop: `1px solid ${MS.ink10}`,
              display: 'flex',
              justifyContent: 'center',
              gap: 20,
            }}
          >
            <Link
              href="/login"
              style={{
                fontSize: 13,
                color: MS.greenDark,
                fontWeight: 500,
                textDecoration: 'none',
              }}
            >
              {t.input.loginLink}
            </Link>
            <Link
              href="/"
              style={{ fontSize: 13, color: MS.ink50, textDecoration: 'none' }}
            >
              {t.input.homeLink}
            </Link>
          </div>
        </div>
      </main>

      <footer
        style={{
          position: 'relative',
          textAlign: 'center',
          padding: '20px',
          fontSize: 12,
          color: MS.ink50,
        }}
      >
        <Link
          href="/privacy"
          style={{ color: MS.ink50, textDecoration: 'none' }}
        >
          {t.input.privacyLink}
        </Link>
      </footer>
    </>
  )
}
