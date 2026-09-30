'use client'

import { Fragment, useEffect, useRef } from 'react'
import Link from 'next/link'
import type { Conversation } from './types'
import { Caret } from './Caret'
import { useT } from '@/lib/i18n/LocaleProvider'
import { homeText } from './translations'

function DemoCard({
  conversation, msgIndex, typedChars, dirigentShown, height, ctaHref, ctaLabel,
}: {
  conversation: Conversation
  msgIndex: number
  typedChars: number
  dirigentShown: boolean
  height: number | string
  ctaHref: string
  ctaLabel: string
}) {
  const t = useT(homeText)
  const feedRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (feedRef.current) feedRef.current.scrollTop = feedRef.current.scrollHeight
  }, [msgIndex, typedChars, dirigentShown])

  const messages = conversation.messages || []
  // dirigent banner is rendered just before the FIRST assistant message
  const firstAssistantIdx = messages.findIndex(m => m.role === 'assistant')

  return (
    <div style={{
      background: 'var(--card)',
      border: '1px solid var(--line)',
      borderRadius: 16,
      overflow: 'hidden',
      boxShadow: '0 12px 32px -8px rgba(13,27,22,0.18)',
      height,
      display: 'flex',
      flexDirection: 'column',
    }}>
      {/* Header */}
      <div style={{
        padding: '10px 14px', borderBottom: '1px solid var(--line)',
        display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0, background: 'var(--card)',
      }}>
        <div style={{ width: 8, height: 8, borderRadius: '50%', background: '#1D9E75' }} />
        <span style={{
          fontSize: 13, fontWeight: 500, color: 'var(--foreground)',
          fontFamily: "'Golos Text', system-ui, sans-serif",
        }}>
          {conversation.name}
        </span>
        <span style={{
          marginLeft: 'auto', fontSize: 11, fontWeight: 500, padding: '2px 7px',
          borderRadius: 4, background: 'color-mix(in oklab, var(--accent) 14%, var(--card))', color: 'var(--accent-strong)', border: '1px solid var(--accent)',
        }}>{t.demoBadge}</span>
      </div>

      {/* Feed — renders every message up to msgIndex; current one is typing */}
      <div
        ref={feedRef}
        style={{
          flex: 1, minHeight: 0, overflowY: 'auto', padding: '12px 14px',
          display: 'flex', flexDirection: 'column', gap: 10, scrollbarWidth: 'none',
        }}
      >
        {messages.map((msg, i) => {
          if (i > msgIndex) return null
          const isTyping = i === msgIndex
          const text = isTyping ? msg.content.slice(0, typedChars) : msg.content
          const showCaret = isTyping && typedChars < msg.content.length
          const showBannerBefore = dirigentShown && i === firstAssistantIdx

          return (
            <Fragment key={i}>
              {showBannerBefore && (
                <div style={{
                  display: 'flex', alignItems: 'flex-start', gap: 10,
                  padding: '10px 12px', borderRadius: 10,
                  background: '#EAF2FB', border: '1px solid #B5CFEA', color: '#2F6EAA',
                  fontSize: 12, lineHeight: 1.45,
                  fontFamily: "'Golos Text', system-ui, sans-serif",
                  animation: 'ms-dir-in 280ms ease',
                }}>
                  <div style={{
                    width: 8, height: 8, borderRadius: '50%',
                    background: '#2F6EAA', marginTop: 5, flexShrink: 0,
                  }} />
                  <div>
                    <span style={{ fontWeight: 600 }}>{t.dirigentName}:</span>{' '}
                    {t.dirigentBanner(conversation.name)}
                  </div>
                </div>
              )}

              {msg.role === 'user' ? (
                <div style={{
                  display: 'flex', flexDirection: 'column', alignItems: 'flex-end',
                  maxWidth: '80%', alignSelf: 'flex-end',
                }}>
                  <div style={{
                    background: '#1D9E75', color: '#fff', padding: '9px 13px',
                    borderRadius: 14, borderBottomRightRadius: 4,
                    fontSize: 14, lineHeight: 1.45, whiteSpace: 'pre-wrap',
                    fontFamily: "'Golos Text', system-ui, sans-serif",
                  }}>
                    {text}{showCaret && <Caret light />}
                  </div>
                </div>
              ) : (
                <div style={{
                  display: 'flex', flexDirection: 'column', alignItems: 'flex-start',
                  maxWidth: '84%', alignSelf: 'flex-start', gap: 4,
                }}>
                  <div style={{
                    fontSize: 11, color: 'var(--muted)',
                    fontFamily: "'Golos Text', system-ui, sans-serif",
                  }}>
                    <span style={{ color: 'var(--accent-strong)', fontWeight: 500 }}>{t.dirigentName}</span>
                  </div>
                  <div style={{
                    background: '#fff', color: '#0F172A', padding: '9px 13px',
                    borderRadius: 14, borderBottomLeftRadius: 4,
                    fontSize: 14, lineHeight: 1.45, whiteSpace: 'pre-wrap',
                    border: '1px solid var(--line)',
                    fontFamily: "'Golos Text', system-ui, sans-serif",
                  }}>
                    {text}{showCaret && <Caret />}
                  </div>
                </div>
              )}
            </Fragment>
          )
        })}
      </div>

      {/* Input row */}
      <div style={{
        padding: '10px 14px', borderTop: '1px solid var(--line)',
        display: 'flex', alignItems: 'center', gap: 8,
        flexShrink: 0, background: 'var(--card)',
      }}>
        <Link
          href={ctaHref}
          style={{
            flex: 1, height: 38, borderRadius: 19,
            background: 'color-mix(in oklab, var(--card) 92%, var(--background))', border: '1px solid var(--line)',
            display: 'flex', alignItems: 'center', padding: '0 14px',
            textDecoration: 'none',
          }}
        >
          <span style={{
            fontSize: 12, color: 'var(--accent-strong)', fontWeight: 500,
            fontFamily: "'Golos Text', system-ui, sans-serif",
          }}>
            {ctaLabel}
          </span>
        </Link>
        <Link
          href="/login"
          style={{
            width: 38, height: 38, borderRadius: '50%', background: '#1D9E75',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            color: '#fff', flexShrink: 0, textDecoration: 'none',
          }}
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none"
            stroke="currentColor" strokeWidth="2.2"
            strokeLinecap="round" strokeLinejoin="round">
            <path d="M8 13V3M3 8l5-5 5 5" />
          </svg>
        </Link>
      </div>
    </div>
  )
}

export { DemoCard }
