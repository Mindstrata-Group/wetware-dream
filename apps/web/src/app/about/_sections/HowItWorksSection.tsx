'use client'

import type { AboutTile } from '../_parts/_content'

export function HowItWorksSection({ howTitleF, tiles, isMobile }: {
  howTitleF: string;
  tiles: AboutTile[];
  isMobile: boolean;
}) {
  return (
    <>
          <section className="ms-about-section" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
            <h2 style={{
              fontFamily: "'Unbounded', system-ui, sans-serif",
              fontWeight: 600,
              fontSize: isMobile ? 18 : 24,
              margin: 0, color: 'var(--foreground)', letterSpacing: '-0.02em',
            }}>
              {howTitleF}
            </h2>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
              {/* tiles from site_content (about.tiles): an admin can add/remove
                  tiles via the CMS. The step number is generated from the position. */}
              {tiles.map((t, i, arr) => {
                const s = { step: String(i + 1).padStart(2, '0'), title: t.title, body: t.body }
                return (
                <div key={i} style={{
                  display: 'flex', gap: 16, alignItems: 'flex-start',
                  paddingBottom: i < arr.length - 1 ? 24 : 0,
                  position: 'relative',
                }}>
                  {/* vertical line */}
                  {i < arr.length - 1 && (
                    <div style={{
                      position: 'absolute',
                      left: 19, top: 40, bottom: 0,
                      width: 1,
                      background: 'rgba(29,158,117,0.2)',
                    }} />
                  )}
                  <div style={{
                    width: 40, height: 40, borderRadius: '50%', flexShrink: 0,
                    background: 'var(--soft)', border: '1px solid #1D9E75',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    fontSize: 11, fontWeight: 700, color: 'var(--foreground)',
                    fontFamily: "'Unbounded', system-ui, sans-serif",
                    position: 'relative', zIndex: 1,
                  }}>{s.step}</div>
                  <div style={{ paddingTop: 8 }}>
                    <div style={{
                      fontSize: 15, fontWeight: 600, color: 'var(--foreground)',
                      fontFamily: "'Golos Text', system-ui, sans-serif",
                      marginBottom: 4,
                    }}>{s.title}</div>
                    <div style={{
                      fontSize: 13, color: 'var(--muted)', lineHeight: 1.55,
                      fontFamily: "'Golos Text', system-ui, sans-serif",
                    }}>{s.body}</div>
                  </div>
                </div>
                )
              })}
            </div>
          </section>

    </>
  );
}
