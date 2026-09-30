'use client'

import type { Comparison } from '../_parts/_content'

export function ComparisonTableSection({ comparisonTitleF, comparisons, isMobile, cmpHeaderAspect, cmpHeaderRegular, cmpHeaderStratum }: {
  comparisonTitleF: string;
  comparisons: Comparison[];
  isMobile: boolean;
  cmpHeaderAspect: string;
  cmpHeaderRegular: string;
  cmpHeaderStratum: string;
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
              {comparisonTitleF}
            </h2>

            <div style={{ overflowX: 'auto', borderRadius: 16, border: '1px solid var(--line)' }}>
              <table className="ms-cmp-table" style={{
                width: '100%', borderCollapse: 'collapse',
                background: 'var(--card)', borderRadius: 16, overflow: 'hidden',
              }}>
                <thead>
                  <tr style={{ background: 'color-mix(in oklab, var(--card) 92%, var(--background))' }}>
                    <th style={{
                      padding: '13px 16px', textAlign: 'left',
                      fontSize: 12, fontWeight: 600, color: 'var(--muted)',
                      borderBottom: '1px solid var(--line)',
                      fontFamily: "'Golos Text', system-ui, sans-serif",
                      width: '24%',
                    }}>{cmpHeaderAspect}</th>
                    <th style={{
                      padding: '13px 16px', textAlign: 'left',
                      fontSize: 12, fontWeight: 600, color: 'var(--muted)',
                      borderBottom: '1px solid var(--line)',
                      fontFamily: "'Golos Text', system-ui, sans-serif",
                      width: '38%',
                    }}>{cmpHeaderRegular}</th>
                    <th style={{
                      padding: '13px 16px', textAlign: 'left',
                      fontSize: 12, fontWeight: 600, color: 'var(--foreground)',
                      borderBottom: '1px solid var(--line)',
                      fontFamily: "'Golos Text', system-ui, sans-serif",
                      width: '38%',
                      background: 'rgba(29,158,117,0.04)',
                    }}>{cmpHeaderStratum}</th>
                  </tr>
                </thead>
                <tbody>
                  {comparisons.map((row, i) => (
                    <tr
                      key={i}
                      className="ms-cmp-row"
                      style={{
                        borderBottom: i < comparisons.length - 1
                          ? '1px solid var(--line)'
                          : 'none',
                        transition: 'background 0.15s',
                      }}
                    >
                      <td style={{
                        padding: '14px 16px',
                        fontSize: 13, fontWeight: 600, color: 'var(--foreground)',
                        verticalAlign: 'top',
                        fontFamily: "'Golos Text', system-ui, sans-serif",
                      }}>{row.aspect}</td>
                      <td style={{
                        padding: '14px 16px',
                        fontSize: 13, color: 'var(--muted)', lineHeight: 1.5,
                        verticalAlign: 'top',
                        fontFamily: "'Golos Text', system-ui, sans-serif",
                      }}>{row.regular}</td>
                      <td style={{
                        padding: '14px 16px',
                        fontSize: 13, color: 'color-mix(in oklab, var(--foreground) 88%, var(--accent) 12%)', lineHeight: 1.5,
                        verticalAlign: 'top',
                        background: 'color-mix(in oklab, var(--accent) 12%, var(--card))',
                        fontFamily: "'Golos Text', system-ui, sans-serif",
                      }}>
                        <span style={{ color: '#1D9E75', marginRight: 5, fontWeight: 700 }}>✓</span>
                        {row.stratum}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

    </>
  );
}
