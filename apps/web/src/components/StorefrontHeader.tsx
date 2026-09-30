'use client'

import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import Link from 'next/link'
import { BrandLogo } from '@/components/BrandLogo'
import { apiFetch } from '@/lib/api'
import { getStorefrontFeatureLinks } from '@/lib/siteContent'
import { useSiteContent } from '@/lib/useSiteContent'
import { useLocale } from '@/lib/i18n/LocaleProvider'
import type { Dictionary } from '@/lib/i18n/types'

const headerText: Dictionary<{ home: string; login: string; profile: string; chat: string; access: string }> = {
  ru: { home: 'Главная', login: 'Войти', profile: 'В профиль', chat: 'В чат', access: 'Получить доступ' },
  en: { home: 'Home', login: 'Log in', profile: 'Profile', chat: 'Chat', access: 'Get access' },
}

type StorefrontProfile = {
  user?: { role?: string; email?: string }
  hasAccess?: boolean
}

type StorefrontHeaderProps = {
  active?: 'blog' | 'shop'
  showBlog?: boolean
  showShop?: boolean
  loginLabel?: string
  chatLabel?: string
  accessLabel?: string
}

export function StorefrontHeader({
  active,
  showBlog = true,
  showShop = true,
  loginLabel,
  chatLabel,
  accessLabel,
}: StorefrontHeaderProps) {
  const locale = useLocale()
  const th = headerText[locale]
  const content = useSiteContent()
  const featureLinks = getStorefrontFeatureLinks(content).filter((link) => {
    if (link.feature === 'blog') return showBlog
    if (link.feature === 'shop') return showShop
    return true
  })
  const [profile, setProfile] = useState<StorefrontProfile | null>(null)

  useEffect(() => {
    let cancelled = false
    apiFetch<StorefrontProfile>('/api/profile')
      .then((data) => {
        if (!cancelled) setProfile(data)
      })
      .catch(() => {
        if (!cancelled) setProfile(null)
      })
    return () => { cancelled = true }
  }, [])

  const role = profile?.user?.role || ''
  const privileged = ['owner', 'admin', 'tester', 'billing_admin', 'content_admin', 'support'].includes(role)
  const accountHref = !profile
    ? '/login'
    : privileged
      ? '/profile'
      : profile.hasAccess
        ? '/chat'
        : '/access?force=promo'
  const accountLabel = !profile
    ? (loginLabel ?? th.login)
    : privileged
      ? th.profile
      : profile.hasAccess
        ? (chatLabel ?? th.chat)
        : (accessLabel ?? th.access)

  return (
    <header style={{ minHeight: 64, display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', padding: '10px 24px', borderBottom: '1px solid var(--line)', background: 'color-mix(in oklab, var(--card) 88%, transparent)', backdropFilter: 'blur(12px)' }}>
      <BrandLogo priority />
      <nav style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap', justifyContent: 'flex-end' }}>
        <Link href="/" style={navButton}>{th.home}</Link>
        {featureLinks.map((link) => (
          <Link key={link.href} href={link.href} style={{ ...navButton, ...(active === link.feature ? activeButton : {}) }}>{link.label}</Link>
        ))}
        <Link href={accountHref} style={{ ...navButton, borderColor: 'var(--accent)', color: active ? 'var(--foreground)' : 'var(--accent-strong)' }}>{accountLabel}</Link>
      </nav>
    </header>
  )
}

const navButton = {
  height: 36,
  padding: '0 14px',
  borderRadius: 8,
  border: '1px solid var(--line)',
  background: 'var(--card)',
  color: 'var(--foreground)',
  display: 'inline-flex',
  alignItems: 'center',
  fontSize: 13,
  fontWeight: 500,
  textDecoration: 'none',
  whiteSpace: 'nowrap',
} satisfies CSSProperties

const activeButton = {
  background: 'var(--accent)',
  borderColor: 'var(--accent)',
  color: '#fff',
} satisfies CSSProperties
