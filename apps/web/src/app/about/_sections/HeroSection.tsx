'use client'

import { Counter } from '../_parts/Counter'
import { useT } from '@/lib/i18n/LocaleProvider'
import { aboutText } from '../translations'

export function HeroSection({ introTitleF, introSubtitleF, isMobile, isFold, modeStats, statsExpertsValueRaw, statsExpertsLabel, statsModesLabel }: {
  introTitleF: string;
  introSubtitleF: string;
  isMobile: boolean;
  isFold: boolean;
  modeStats: { total: number; paid: number };
  statsExpertsValueRaw: string;
  statsExpertsLabel: string;
  statsModesLabel: string;
}) {
  const t = useT(aboutText)
  return (
    <>
          <section className="ms-about-section" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div style={{
              display: 'inline-flex', alignItems: 'center', gap: 8,
              padding: '5px 12px', borderRadius: 999,
              background: 'var(--soft)', border: '1px solid #1D9E75',
              width: 'fit-content',
            }}>
              <div style={{ width: 7, height: 7, borderRadius: '50%', background: '#1D9E75' }} />
              <span style={{
                fontSize: 12, fontWeight: 600, color: 'var(--foreground)',
                fontFamily: "'Golos Text', system-ui, sans-serif",
              }}>{t.badge}</span>
            </div>

            <h1 style={{
              fontFamily: "'Unbounded', system-ui, sans-serif",
              fontWeight: 600,
              fontSize: isFold ? 22 : isMobile ? 28 : 46,
              lineHeight: 1.06,
              letterSpacing: '-0.03em',
              margin: 0,
              color: 'var(--foreground)',
              maxWidth: 720,
            }}>
              {introTitleF}
            </h1>

            <p style={{
              fontSize: isMobile ? 15 : 18,
              color: 'var(--muted)',
              lineHeight: 1.55,
              margin: 0,
              maxWidth: 640,
            }}>
              {introSubtitleF}
            </p>

            {/* stats row */}
            <div style={{
              display: 'flex',
              flexWrap: 'wrap',
              gap: isMobile ? 12 : 20,
              marginTop: 8,
            }}>
              {(() => {
                // parse "100%" → { n: 100, suffix: '%' }; "5+" → { n: 5, suffix: '+' }; etc.
                const m = statsExpertsValueRaw.match(/^(\d+)(.*)$/)
                const expertsN = m ? parseInt(m[1], 10) : 100
                const expertsSuffix = m ? m[2] : ''
                return [
                  { n: modeStats.paid, suffix: '', label: statsModesLabel },
                  { n: expertsN, suffix: expertsSuffix, label: statsExpertsLabel },
                ]
              })().map(s => (
                <div key={s.label} style={{
                  display: 'flex', flexDirection: 'column', gap: 2,
                  padding: '14px 20px', borderRadius: 14,
                  background: 'color-mix(in oklab, var(--card) 82%, transparent)',
                  border: '1px solid var(--line)',
                  minWidth: 110,
                }}>
                  <span style={{
                    fontSize: isMobile ? 28 : 36,
                    fontWeight: 700,
                    fontFamily: "'Unbounded', system-ui, sans-serif",
                    color: 'var(--foreground)',
                    lineHeight: 1,
                  }}>
                    <Counter to={s.n} suffix={s.suffix} />
                  </span>
                  <span style={{ fontSize: 12, color: 'var(--muted)' }}>{s.label}</span>
                </div>
              ))}
            </div>
          </section>

    </>
  );
}
