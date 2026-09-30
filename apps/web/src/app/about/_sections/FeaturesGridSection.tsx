'use client'

import type { Feature } from '../_parts/_content'

export function FeaturesGridSection({ featuresTitleF, features, isMobile }: {
  featuresTitleF: string;
  features: Feature[];
  isMobile: boolean;
}) {
  return (
    <>
          <section className="ms-about-section" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
            <h2 style={{
              fontFamily: "'Unbounded', system-ui, sans-serif",
              fontWeight: 600,
              fontSize: isMobile ? 18 : 24,
              margin: 0,
              color: 'var(--foreground)',
              letterSpacing: '-0.02em',
            }}>
              {featuresTitleF}
            </h2>

            <div
              className="ms-feat-grid"
              style={{
                display: 'grid',
                gridTemplateColumns: isMobile ? '1fr' : 'repeat(3, 1fr)',
                gap: 14,
              }}
            >
              {features.map((f, i) => (
                <div key={i} style={{
                  padding: '20px 18px',
                  borderRadius: 16,
                  background: 'color-mix(in oklab, var(--card) 86%, transparent)',
                  border: '1px solid var(--line)',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 10,
                }}>
                  <div style={{ fontSize: 26 }}>{f.icon}</div>
                  <div style={{
                    fontSize: 15, fontWeight: 600, color: 'var(--foreground)',
                    fontFamily: "'Golos Text', system-ui, sans-serif",
                  }}>{f.title}</div>
                  <div style={{
                    fontSize: 13, color: 'var(--muted)', lineHeight: 1.55,
                    fontFamily: "'Golos Text', system-ui, sans-serif",
                  }}>{f.body}</div>
                </div>
              ))}
            </div>
          </section>

    </>
  );
}
