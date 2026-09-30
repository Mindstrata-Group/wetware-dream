'use client'

import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import Link from 'next/link'

// Renders legal documents from the CMS. Full GitHub-flavored Markdown support:
// H1-H6 headings, bold/italic/strikethrough, numbered and bulleted
// lists, tables, blockquote, code, links, images, checklists.
export function EditableLegalBody({ body }: { body: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        h1: ({ children }) => (
          <h1 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 26, fontWeight: 600, margin: '40px 0 16px', letterSpacing: '-0.02em' }}>{children}</h1>
        ),
        h2: ({ children }) => (
          <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 20, fontWeight: 600, margin: '32px 0 12px', letterSpacing: '-0.01em' }}>{children}</h2>
        ),
        h3: ({ children }) => (
          <h3 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 16, fontWeight: 600, margin: '24px 0 10px' }}>{children}</h3>
        ),
        h4: ({ children }) => (
          <h4 style={{ fontSize: 15, fontWeight: 600, margin: '20px 0 8px' }}>{children}</h4>
        ),
        p: ({ children }) => (
          <p style={{ margin: '0 0 14px', lineHeight: 1.75 }}>{children}</p>
        ),
        a: ({ href, children }) => {
          const url = String(href ?? '#')
          const isInternal = url.startsWith('/') || url.startsWith('#')
          if (isInternal) {
            return (
              <Link href={url} style={{ color: 'var(--accent-strong)', borderBottom: '1px dashed rgba(29,158,117,0.4)', textDecoration: 'none' }}>
                {children}
              </Link>
            )
          }
          return (
            <a href={url} target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent-strong)', borderBottom: '1px dashed rgba(29,158,117,0.4)', textDecoration: 'none' }}>
              {children}
            </a>
          )
        },
        ul: ({ children }) => (
          <ul style={{ paddingLeft: 22, margin: '0 0 14px', display: 'flex', flexDirection: 'column', gap: 4 }}>{children}</ul>
        ),
        ol: ({ children }) => (
          <ol style={{ paddingLeft: 22, margin: '0 0 14px', display: 'flex', flexDirection: 'column', gap: 4 }}>{children}</ol>
        ),
        li: ({ children }) => (
          <li style={{ lineHeight: 1.65 }}>{children}</li>
        ),
        strong: ({ children }) => (
          <strong style={{ fontWeight: 600 }}>{children}</strong>
        ),
        em: ({ children }) => (
          <em style={{ fontStyle: 'italic' }}>{children}</em>
        ),
        del: ({ children }) => (
          <del style={{ opacity: 0.6 }}>{children}</del>
        ),
        blockquote: ({ children }) => (
          <blockquote style={{ borderLeft: '3px solid var(--accent)', paddingLeft: 14, margin: '16px 0', color: 'var(--muted)', fontStyle: 'italic' }}>{children}</blockquote>
        ),
        code: ({ children, className }) => {
          const isBlock = className?.includes('language-')
          if (isBlock) {
            return <code style={{ display: 'block', padding: 14, borderRadius: 8, background: 'var(--soft)', fontFamily: 'ui-monospace, monospace', fontSize: 13, overflow: 'auto', margin: '12px 0' }}>{children}</code>
          }
          return <code style={{ padding: '1px 6px', borderRadius: 4, background: 'var(--soft)', fontFamily: 'ui-monospace, monospace', fontSize: 13 }}>{children}</code>
        },
        pre: ({ children }) => (
          <pre style={{ margin: '12px 0', overflow: 'auto' }}>{children}</pre>
        ),
        table: ({ children }) => (
          <div style={{ overflowX: 'auto', margin: '16px 0', borderRadius: 10, border: '1px solid var(--line)' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>{children}</table>
          </div>
        ),
        th: ({ children }) => (
          <th style={{ padding: '10px 14px', textAlign: 'left', borderBottom: '1px solid var(--line)', background: 'var(--soft)', fontWeight: 600, fontSize: 13 }}>{children}</th>
        ),
        td: ({ children }) => (
          <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--line)', verticalAlign: 'top' }}>{children}</td>
        ),
        hr: () => (
          <hr style={{ border: 'none', borderTop: '1px solid var(--line)', margin: '24px 0' }} />
        ),
        img: ({ src, alt }) => (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={String(src ?? '')} alt={alt ?? ''} style={{ maxWidth: '100%', borderRadius: 8, margin: '12px 0' }} />
        ),
      }}
    >
      {body}
    </ReactMarkdown>
  )
}
