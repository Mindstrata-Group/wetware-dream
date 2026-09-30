'use client'

import { useCallback, useEffect, useState } from 'react'
import { CookieBanner } from '@/components/CookieBanner'
import { apiFetch } from '@/lib/api'
import { getStorefrontFeatureLinks, getString } from '@/lib/siteContent'
import { useSiteContent } from '@/lib/useSiteContent'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'

// SOLID-split 2026-05-31: types/constants/utils + home sections moved to _home/
import type { ApiModeStats, Conversation, DemoMode, HomeProfile } from './_home/types'
import { FALLBACK_CONVERSATIONS, FALLBACK_PAID, FALLBACK_TOTAL } from './_home/constants'
import { homeText } from './_home/translations'
import { parseDemoChat } from './_home/utils'
import { DemoSection } from './_home/DemoSection'
import { DotsBackground } from './_home/DotsBackground'
import { HeroSection } from './_home/HeroSection'
import { MobileCta } from './_home/MobileCta'
import { TopBar } from './_home/TopBar'
import { useDemoRotation } from './_home/useDemoRotation'
import { OPERATOR_FOOTER_RU } from '@/lib/operator'

export default function HomePage() {
  const locale = useLocale()
  const t = useT(homeText)
  const content = useSiteContent()
  const heroTitleRaw = getString(content, 'landing.hero.title', 'Опишите задачу — Стратум сам выберет режим')
  const heroSubtitleRaw = getString(content, 'landing.hero.subtitle', 'Внутри много режимов для текста, решений, переговоров и сложных ситуаций. Пишите обычным языком: Стратум поймёт задачу, переключит режим и продолжит разбор.')
  const ctaLabelRaw = getString(content, 'landing.hero.cta_label', 'Посмотреть, как это работает')
  const ctaHref = getString(content, 'landing.hero.cta_href', '/about')
  const footerCopyrightRaw = getString(content, 'landing.footer.copyright', OPERATOR_FOOTER_RU)
  const ctaPrimaryLabelRaw = getString(content, 'landing.cta.primary_label', 'Разобрать свою задачу')
  const legalLinkLabelRaw = getString(content, 'landing.legal_link_label', 'Правовая информация')

  const [profileRole, setProfileRole] = useState<string | null>(null)
  const [profileHasAccess, setProfileHasAccess] = useState(false)
  const [demos, setDemos] = useState<Conversation[]>(FALLBACK_CONVERSATIONS)
  const [modeStats, setModeStats] = useState<ApiModeStats>({ paid: FALLBACK_PAID, total: FALLBACK_TOTAL })

  const [isMobile, setIsMobile] = useState(false)
  const [isFold, setIsFold] = useState(false)
  const [isDesktop, setIsDesktop] = useState(true)

  const { active, msgIndex, typedChars, dirigentShown, pickDemo } = useDemoRotation(demos)

  const contentVars = { modes_count: String(modeStats.total || modeStats.paid) }
  const fillContentVars = (value: string) => value.replace(/{{\s*modes_count\s*}}/g, contentVars.modes_count)
  const normalizeLandingCopy = (key: string, value: string) => {
    const legacy: Record<string, Record<string, string>> = {
      'landing.hero.title': {
        'Сам поймёт — включит нужный режим': 'Опишите задачу — Стратум сам выберет режим',
        'Разберите рабочую ситуацию в чате': 'Опишите задачу — Стратум сам выберет режим',
      },
      'landing.hero.subtitle': {
        'Один чат — 14 точек зрения на любую задачу.': 'Внутри {{modes_count}} режимов для текста, решений, переговоров и сложных ситуаций. Пишите обычным языком: Стратум поймёт задачу, переключит режим и продолжит разбор.',
        'Один чат — {{modes_count}} точек зрения на любую задачу.': 'Внутри {{modes_count}} режимов для текста, решений, переговоров и сложных ситуаций. Пишите обычным языком: Стратум поймёт задачу, переключит режим и продолжит разбор.',
        'Опишите задачу своими словами. Стратум задаст уточняющие вопросы, выберет подходящий ход разбора и соберёт следующий шаг.': 'Внутри {{modes_count}} режимов для текста, решений, переговоров и сложных ситуаций. Пишите обычным языком: Стратум поймёт задачу, переключит режим и продолжит разбор.',
        'Внутри много режимов для текста, решений, переговоров и сложных ситуаций. Пишите обычным языком: система поймёт задачу, переключит режим и продолжит разбор.': 'Внутри {{modes_count}} режимов для текста, решений, переговоров и сложных ситуаций. Пишите обычным языком: Стратум поймёт задачу, переключит режим и продолжит разбор.',
      },
      'landing.hero.cta_label': {
        'Узнать, чем мы отличаемся от обычной нейросети →': 'Посмотреть, как это работает',
      },
      'landing.cta.primary_label': {
        'Начать пользоваться': 'Разобрать свою задачу',
      },
    }
    return legacy[key]?.[value] ?? value
  }
  // The EN version does not read RU content from the CMS; it takes ready texts from the dictionary.
  const heroTitle = locale === 'en' ? t.hero.title : fillContentVars(normalizeLandingCopy('landing.hero.title', heroTitleRaw))
  const heroSubtitle = locale === 'en' ? fillContentVars(t.hero.subtitle) : fillContentVars(normalizeLandingCopy('landing.hero.subtitle', heroSubtitleRaw))
  const ctaLabel = locale === 'en' ? t.hero.ctaLabel : fillContentVars(normalizeLandingCopy('landing.hero.cta_label', ctaLabelRaw))
  const footerCopyright = locale === 'en' ? t.footerCopyright : fillContentVars(footerCopyrightRaw)
  const ctaPrimaryLabel = locale === 'en' ? t.hero.ctaPrimaryLabel : fillContentVars(normalizeLandingCopy('landing.cta.primary_label', ctaPrimaryLabelRaw))
  const legalLinkLabel = locale === 'en' ? t.hero.legalLinkLabel : fillContentVars(legalLinkLabelRaw)
  const featureLinksRaw = getStorefrontFeatureLinks(content)
  const featureLinks = locale === 'en'
    ? featureLinksRaw.map((link) => ({ href: link.href, label: link.feature === 'blog' ? t.featureLinks.blog : t.featureLinks.shop }))
    : featureLinksRaw

  useEffect(() => {
    const check = () => {
      const w = window.innerWidth
      setIsFold(w < 390)
      setIsMobile(w < 768)
      setIsDesktop(w >= 1024)
    }
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])

  useEffect(() => {
    let cancelled = false
    apiFetch<HomeProfile>('/api/profile')
      .then(j => {
        if (cancelled) return
        setProfileRole(j.user.role)
        setProfileHasAccess(Boolean(j.hasAccess))
      })
      .catch(() => {
        if (cancelled) return
        setProfileRole(null)
        setProfileHasAccess(false)
      })
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    let cancelled = false

    const loadDemoModes = fetch('/api/public/demo-modes')
      .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
      .then(j => {
        if (cancelled) return
        const demoModes: DemoMode[] = Array.isArray(j?.modes) ? j.modes : []
        if (!demoModes.length) return

        setModeStats({ total: demoModes.length, paid: demoModes.length })

        const built: Conversation[] = demoModes
          .map((m: DemoMode) => ({ name: m.name, messages: parseDemoChat(m) }))
          .filter(c => c.messages.length >= 2)

        if (built.length >= 1) {
          const shuffled = [...built].sort(() => Math.random() - 0.5)
          setDemos(shuffled.slice(0, 8))
        }
      })
      .catch(err => {
        console.warn('[home] demo-modes fetch failed:', err)
      })

    void Promise.allSettled([loadDemoModes])
    return () => { cancelled = true }
  }, [locale])

  const logout = async () => {
    await fetch('/api/auth/logout', { method: 'POST', credentials: 'include' }).catch(() => {})
    window.location.href = '/'
  }

  const privilegedRole = !!profileRole && ['owner', 'admin', 'tester'].includes(profileRole)
  const entryHref = !profileRole
    ? '/login'
    : privilegedRole
      ? '/profile'
      : profileHasAccess
        ? '/chat'
        : '/access?force=promo'
  const entryLabel = !profileRole
    ? t.entry.login
    : privilegedRole
      ? t.entry.profile
      : profileHasAccess
        ? t.entry.chat
        : t.entry.access

  const topbarH = isMobile ? 54 : 68
  const padH = isFold ? 14 : isMobile ? 16 : 64
  const h1Size = isFold ? 22 : isMobile ? 26 : isDesktop ? 44 : 34

  const stripDemos = demos.reduce((acc: typeof demos, d) => {
    if (!acc.find(x => x.name === d.name)) acc.push(d)
    return acc
  }, [])
  const activeStrip = stripDemos.findIndex(d => d.name === demos[active]?.name)

  const onStripPick = useCallback((i: number) => {
    const targetName = stripDemos[i]?.name
    const idx = demos.findIndex(d => d.name === targetName)
    if (idx >= 0) pickDemo(idx)
  }, [stripDemos, demos, pickDemo])

  return (
    <>
      <style>{`
        @keyframes ms-blink { 0%,50%{opacity:1} 50.01%,100%{opacity:0} }
        @keyframes ms-dir-in { from{transform:translateY(-6px);opacity:0} to{transform:none;opacity:1} }
      `}</style>

      <div style={{
        height: '100dvh',
        overflow: 'hidden',
        position: 'relative',
        background: 'var(--background)',
        color: 'var(--foreground)',
        fontFamily: "'Golos Text', system-ui, sans-serif",
        display: 'flex',
        flexDirection: 'column',
      }}>
        <div style={{ position: 'absolute', inset: 0, zIndex: 0 }}>
          <DotsBackground />
        </div>

        <TopBar
          topbarH={topbarH}
          padH={padH}
          isFold={isFold}
          isMobile={isMobile}
          entryHref={entryHref}
          entryLabel={entryLabel}
          profileRole={profileRole}
          featureLinks={featureLinks}
          onLogout={logout}
        />

        <div style={{
          position: 'relative', zIndex: 1,
          flex: 1, minHeight: 0,
          display: 'flex',
          flexDirection: isDesktop ? 'row' : 'column',
          gap: isDesktop ? 56 : isFold ? 8 : 12,
          padding: isDesktop
            ? `clamp(18px,3vh,44px) ${padH}px clamp(14px,2.5vh,28px)`
            : `${isFold ? 8 : 12}px ${padH}px 0`,
          alignItems: isDesktop ? 'center' : 'stretch',
          overflow: 'hidden',
        }}>
          <HeroSection
            heroTitle={heroTitle}
            heroSubtitle={heroSubtitle}
            ctaPrimaryLabel={ctaPrimaryLabel}
            legalLinkLabel={legalLinkLabel}
            isFold={isFold}
            isMobile={isMobile}
            isDesktop={isDesktop}
            h1Size={h1Size}
            stripDemos={stripDemos}
            activeStrip={activeStrip}
            onStripPick={onStripPick}
          />

          <DemoSection
            activeConversation={demos[active]}
            msgIndex={msgIndex}
            typedChars={typedChars}
            dirigentShown={dirigentShown}
            isDesktop={isDesktop}
            ctaHref={ctaHref}
            ctaLabel={ctaLabel}
          />
        </div>

        {!isDesktop && (
          <MobileCta
            isFold={isFold}
            ctaPrimaryLabel={ctaPrimaryLabel}
            legalLinkLabel={legalLinkLabel}
            footerCopyright={footerCopyright}
          />
        )}
        {isDesktop && (
          <div style={{
            position: 'absolute', bottom: 8, left: 0, right: 0, textAlign: 'center',
            fontSize: 11, color: 'var(--muted)',
            fontFamily: "'Golos Text', system-ui, sans-serif", opacity: 0.7,
            pointerEvents: 'none', zIndex: 1,
          }}>
            {footerCopyright}
          </div>
        )}
      </div>
      <CookieBanner />
    </>
  )
}
