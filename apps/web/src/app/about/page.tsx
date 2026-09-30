'use client'

import { useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { BrandLogo } from '@/components/BrandLogo'
import { useSiteContent } from '@/lib/useSiteContent'
import { getString, getArray, fillVars } from '@/lib/siteContent'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { aboutText } from './translations'

// SOLID-split 2026-05-31: types/fallbacks/animated counter moved to _parts/
import type {
  AboutTile, Comparison, Feature, ApiMode, ApiModeStats,
} from './_parts/_content'
import {
  FALLBACK_TILES, FALLBACK_TOTAL, FALLBACK_PAID,
  COMPARISONS, FEATURES,
} from './_parts/_content'
import { Counter } from './_parts/Counter'

// SOLID-split 2026-05-31: 5 sections (Hero, Comparison, Features, How, CTA) moved out.
import { HeroSection } from './_sections/HeroSection'
import { ComparisonTableSection } from './_sections/ComparisonTableSection'
import { FeaturesGridSection } from './_sections/FeaturesGridSection'
import { HowItWorksSection } from './_sections/HowItWorksSection'
import { CtaSection } from './_sections/CtaSection'
import { OPERATOR_FOOTER_RU } from '@/lib/operator'

// ── Animated counter ──────────────────────────────────────────────────────────
// (removed: _parts/Counter)

// ── Page ──────────────────────────────────────────────────────────────────────
export default function AboutPage() {
  const locale = useLocale()
  const t = useT(aboutText)
  const content = useSiteContent()
  const introTitle = locale === 'en' ? t.introTitle : getString(content, 'about.intro.title', 'Не просто нейросеть — платформа профессиональных режимов')
  const introSubtitle = locale === 'en' ? t.introSubtitle : getString(content, 'about.intro.subtitle', 'Обычная нейросеть — универсальный инструмент без фокуса. СТРАТУМ — это набор специализированных методологий, которые сами выбирают подход под вашу задачу.')
  const comparisonTitle = locale === 'en' ? t.comparisonTitle : getString(content, 'about.comparison.title', 'Сравнение с обычной нейросетью')
  const comparisons = locale === 'en' ? t.comparisons : getArray<Comparison>(content, 'about.comparisons', COMPARISONS)
  const featuresTitle = locale === 'en' ? t.featuresTitle : getString(content, 'about.features.title', 'Что внутри')
  const features = locale === 'en' ? t.features : getArray<Feature>(content, 'about.features', FEATURES)
  const howTitle = locale === 'en' ? t.howTitle : getString(content, 'about.how.title', 'Как это работает')
  const statsModesLabel = locale === 'en' ? t.stats.modesLabel : getString(content, 'about.stats.modes_label', 'режимов')
  const statsExpertsValueRaw = locale === 'en' ? t.stats.expertsValue : getString(content, 'about.stats.experts_value', '100%')
  const statsExpertsLabel = locale === 'en' ? t.stats.expertsLabel : getString(content, 'about.stats.experts_label', 'инструкции от экспертов')
  const cmpHeaderAspect = locale === 'en' ? t.comparisonHeaders.aspect : getString(content, 'about.comparison.header_aspect', 'Аспект')
  const cmpHeaderRegular = locale === 'en' ? t.comparisonHeaders.regular : getString(content, 'about.comparison.header_regular', 'Обычная нейросеть')
  const cmpHeaderStratum = locale === 'en' ? t.comparisonHeaders.stratum : getString(content, 'about.comparison.header_stratum', 'СТРАТУМ')
  const ctaTitle = locale === 'en' ? t.ctaTitle : getString(content, 'about.hero.title', 'Попробуйте прямо сейчас')
  const ctaSubtitle = locale === 'en' ? t.ctaSubtitle : getString(content, 'about.hero.subtitle', 'Первый режим бесплатно — просто введите промокод или войдите через Яндекс.')
  const tiles = locale === 'en' ? t.tiles : getArray<AboutTile>(content, 'about.tiles', FALLBACK_TILES)
  const footerCopyright = locale === 'en' ? t.footer.copyright : getString(content, 'landing.footer.copyright', OPERATOR_FOOTER_RU)

  const [isMobile, setIsMobile] = useState(false)
  const [isFold, setIsFold] = useState(false)
  const [modeStats, setModeStats] = useState<ApiModeStats>({ total: FALLBACK_TOTAL, paid: FALLBACK_PAID })
  // {{modes_count}} etc. are substituted into any admin-editable text.
  const vars = { modes_count: modeStats.total || modeStats.paid, paid_count: modeStats.paid, total_count: modeStats.total }
  const introTitleF = fillVars(introTitle, vars)
  const introSubtitleF = fillVars(introSubtitle, vars)
  const comparisonTitleF = fillVars(comparisonTitle, vars)
  const featuresTitleF = fillVars(featuresTitle, vars)
  const howTitleF = fillVars(howTitle, vars)
  const ctaTitleF = fillVars(ctaTitle, vars)
  const ctaSubtitleF = fillVars(ctaSubtitle, vars)
  const footerCopyrightF = fillVars(footerCopyright, vars)

  useEffect(() => {
    const check = () => {
      setIsFold(window.innerWidth < 390)
      setIsMobile(window.innerWidth < 768)
    }
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])

  const padH = isFold ? 14 : isMobile ? 18 : 64


	useEffect(() => {
			let cancelled = false

			fetch('/api/public/demo-modes')
			  .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
			  .then((json) => {
				if (cancelled) return
				const allModes: ApiMode[] = Array.isArray(json?.modes) ? json.modes : []
				const total = allModes.length
				if (total <= 0) return

				setModeStats({ total, paid: total })
			  })
			  .catch(() => {})

			return () => { cancelled = true }
		  }, [])
	  
  return (
    <>
      <style>{`
        @keyframes ms-fade-up {
          from { opacity: 0; transform: translateY(18px); }
          to   { opacity: 1; transform: none; }
        }
        .ms-about-section { animation: ms-fade-up 0.5s ease both; }
        .ms-about-section:nth-child(2) { animation-delay: 0.07s; }
        .ms-about-section:nth-child(3) { animation-delay: 0.14s; }
        .ms-about-section:nth-child(4) { animation-delay: 0.21s; }
        .ms-about-section:nth-child(5) { animation-delay: 0.28s; }
        .ms-cmp-row:hover { background: rgba(29,158,117,0.04); }
        @media (max-width: 640px) {
          .ms-cmp-table th, .ms-cmp-table td { font-size: 12px; padding: 10px 9px; }
          .ms-feat-grid { grid-template-columns: 1fr !important; }
        }
      `}</style>

      <div style={{
        minHeight: '100dvh',
        background: 'var(--background)',
        color: 'var(--foreground)',
        fontFamily: "'Golos Text', system-ui, sans-serif",
        display: 'flex',
        flexDirection: 'column',
      }}>

        {/* ── TOPBAR ────────────────────────────────────────────────────── */}
        <header style={{
          position: 'sticky', top: 0, zIndex: 20,
          height: isMobile ? 54 : 68, flexShrink: 0,
          display: 'flex', alignItems: 'center',
          justifyContent: 'space-between',
          padding: `0 ${padH}px`,
          background: 'rgba(var(--card-rgb), 0.88)',
          backdropFilter: 'blur(12px)',
          borderBottom: '1px solid var(--line)',
        }}>
          <BrandLogo className="ms-brand-link" />
          <nav style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <Link href="/" style={{
              height: isMobile ? 34 : 40, padding: '0 15px', borderRadius: 10,
              border: '1px solid var(--line)', background: 'rgba(var(--card-rgb), 0.88)',
              color: 'var(--foreground)', fontSize: 13, fontWeight: 500,
              display: 'inline-flex', alignItems: 'center', textDecoration: 'none',
              fontFamily: "'Golos Text', system-ui, sans-serif", boxShadow: 'none',
            }}>{t.nav.home}</Link>
            <Link href="/access?force=promo" style={{
              height: isMobile ? 34 : 40, padding: '0 15px', borderRadius: 10,
              border: '1px solid #1D9E75',
              background: 'linear-gradient(160deg, #22b09a 0%, #1D9E75 100%)',
              color: '#fff', fontSize: 13, fontWeight: 500,
              display: 'inline-flex', alignItems: 'center', textDecoration: 'none',
              fontFamily: "'Golos Text', system-ui, sans-serif", boxShadow: 'none',
            }}>{t.nav.tryIt}</Link>
          </nav>
        </header>

        {/* ── MAIN ──────────────────────────────────────────────────────── */}
        <main style={{
          flex: 1,
          maxWidth: 960,
          width: '100%',
          margin: '0 auto',
          padding: `${isMobile ? 28 : 52}px ${padH}px ${isMobile ? 32 : 64}px`,
          display: 'flex',
          flexDirection: 'column',
          gap: isMobile ? 40 : 60,
        }}>

          {/* ── HERO ────────────────────────────────────────────────────── */}
          <HeroSection introTitleF={introTitleF} introSubtitleF={introSubtitleF} isMobile={isMobile} isFold={isFold} modeStats={modeStats} statsExpertsValueRaw={statsExpertsValueRaw} statsExpertsLabel={statsExpertsLabel} statsModesLabel={statsModesLabel} />
          {/* ── COMPARISON TABLE ──────────────────────────────────────────── */}
          <ComparisonTableSection comparisonTitleF={comparisonTitleF} comparisons={comparisons} isMobile={isMobile} cmpHeaderAspect={cmpHeaderAspect} cmpHeaderRegular={cmpHeaderRegular} cmpHeaderStratum={cmpHeaderStratum} />
          {/* ── FEATURES GRID ────────────────────────────────────────────── */}
          <FeaturesGridSection featuresTitleF={featuresTitleF} features={features} isMobile={isMobile} />
          {/* ── HOW IT WORKS ─────────────────────────────────────────────── */}
          <HowItWorksSection howTitleF={howTitleF} tiles={tiles} isMobile={isMobile} />
          {/* ── CTA ──────────────────────────────────────────────────────── */}
          <CtaSection ctaTitleF={ctaTitleF} ctaSubtitleF={ctaSubtitleF} isMobile={isMobile} />
        </main>

        {/* ── FOOTER ────────────────────────────────────────────────────── */}
        <footer style={{
          borderTop: '1px solid var(--line)',
          padding: `12px ${padH}px`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 16,
          flexWrap: 'wrap',
        }}>
          <span style={{
            fontSize: 12, color: 'var(--muted)',
            fontFamily: "'Golos Text', system-ui, sans-serif",
          }}>{footerCopyrightF}</span>
          {[
            { href: '/', label: t.footer.links.home },
            { href: '/privacy', label: t.footer.links.privacy },
            { href: '/access?force=promo', label: t.footer.links.access },
          ].map(l => (
            <Link key={l.href} href={l.href} style={{
              fontSize: 12, color: 'var(--muted)', textDecoration: 'none',
              fontFamily: "'Golos Text', system-ui, sans-serif",
            }}>{l.label}</Link>
          ))}
        </footer>
      </div>
    </>
  )
}
