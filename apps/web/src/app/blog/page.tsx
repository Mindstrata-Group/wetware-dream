'use client'

import { useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import Link from 'next/link'
import { StorefrontHeader } from '@/components/StorefrontHeader'
import { blogPostCoverUrl, type BlogPostContent, getPublishedBlogPosts } from '@/lib/blogContent'
import { getBoolean, getString } from '@/lib/siteContent'
import { useSiteContent } from '@/lib/useSiteContent'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { blogText } from './translations'

export default function BlogPage() {
  const content = useSiteContent()
  const locale = useLocale()
  const t = useT(blogText)
  const enabled = getBoolean(content, 'features.blog_enabled', false)
  const title = locale === 'en' ? t.hero.title : getString(content, 'blog.hero.title', 'Блог Mindstrata')
  const subtitle = locale === 'en' ? t.hero.subtitle : getString(content, 'blog.hero.subtitle', 'Короткие разборы методик, выступлений и сценариев применения ИИ.')
  const closedTitle = locale === 'en' ? t.closed.title : getString(content, 'blog.closed.title', 'Блог пока закрыт')
  const closedText = locale === 'en' ? t.closed.text : getString(content, 'blog.closed.text', 'Раздел скоро откроется.')
  const emptyState = locale === 'en' ? t.emptyState : getString(content, 'blog.empty_state', 'В этой теме пока нет опубликованных статей.')
  const posts = getPublishedBlogPosts(content, locale)
  const [activeTag, setActiveTag] = useState(t.allTag)
  const [page, setPage] = useState(1)
  const pageSize = 9
  const tags = useMemo(() => [t.allTag, ...Array.from(new Set(posts.map((p) => p.tag).filter(Boolean)))], [posts, t.allTag])
  const filteredPosts = useMemo(
    () => activeTag === t.allTag ? posts : posts.filter((post) => post.tag === activeTag),
    [activeTag, posts, t.allTag],
  )
  const totalPages = Math.max(1, Math.ceil(filteredPosts.length / pageSize))
  const pageSafe = Math.min(page, totalPages)
  const pagedPosts = filteredPosts.slice((pageSafe - 1) * pageSize, pageSafe * pageSize)
  const [hero, ...rest] = pagedPosts

  if (!enabled) return <ClosedSection title={closedTitle} text={closedText} />

  return (
    <main style={{ minHeight: '100dvh', background: 'var(--background)', color: 'var(--foreground)', fontFamily: 'var(--font-golos)' }}>
      <StorefrontHeader active="blog" />
      <section style={{ maxWidth: 1120, margin: '0 auto', padding: '42px 24px 72px' }}>
        <div style={{ maxWidth: 780 }}>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-golos)', fontSize: 'clamp(30px, 5vw, 50px)', lineHeight: 1.04, fontWeight: 800, letterSpacing: 0 }}>
            {title}
          </h1>
          <p style={{ margin: '14px 0 0', color: 'var(--muted)', fontSize: 17, lineHeight: 1.55 }}>
            {subtitle}
          </p>
        </div>

        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 26 }}>
          {tags.map((tag) => (
            <button
              key={tag}
              type="button"
              onClick={() => {
                setActiveTag(tag)
                setPage(1)
              }}
              style={tag === activeTag ? activeTagButton : tagButton}
            >
              {tag}
            </button>
          ))}
        </div>

        {hero && <HeroPost post={hero} />}

        {rest.length > 0 && (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 14, marginTop: 24 }}>
            {rest.map((post) => <PostCard key={post.slug} post={post} />)}
          </div>
        )}

        {filteredPosts.length === 0 && (
          <div style={{ marginTop: 32, padding: 20, border: '1px solid var(--line)', borderRadius: 8, background: 'var(--card)', color: 'var(--muted)' }}>
            {emptyState}
          </div>
        )}

        {filteredPosts.length > pageSize && (
          <nav aria-label={t.paginationLabel} style={{ display: 'flex', justifyContent: 'center', gap: 8, marginTop: 26, flexWrap: 'wrap' }}>
            {Array.from({ length: totalPages }, (_, idx) => idx + 1).map((num) => (
              <button
                key={num}
                type="button"
                onClick={() => setPage(num)}
                aria-current={num === pageSafe ? 'page' : undefined}
                style={num === pageSafe ? activePageButton : pageButton}
              >
                {num}
              </button>
            ))}
          </nav>
        )}

        <SubscribeStrip
          title={locale === 'en' ? t.subscribe.title : getString(content, 'blog.subscribe.title', 'Раз в две недели — один полезный разбор')}
          text={locale === 'en' ? t.subscribe.text : getString(content, 'blog.subscribe.text', 'Без новостной ленты. Только методика, сценарий или практический вывод.')}
          buttonLabel={locale === 'en' ? t.subscribe.buttonLabel : getString(content, 'blog.subscribe.button_label', 'Войти и попробовать')}
        />
      </section>
    </main>
  )
}

function HeroPost({ post }: { post: BlogPostContent }) {
  const t = useT(blogText)
  return (
    <Link href={`/blog/${encodeURIComponent(post.slug)}`} style={{ ...cardLink, marginTop: 26 }}>
      <article style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)', border: '1px solid var(--line)', borderRadius: 8, background: 'var(--card)', overflow: 'hidden' }} className="ms-blog-hero-card">
        <Cover post={post} minHeight={280} label="hero" />
        <div style={{ padding: 26, display: 'flex', flexDirection: 'column', justifyContent: 'center', gap: 11 }}>
          <Meta post={post} tone="strong" />
          <h2 style={{ margin: 0, fontFamily: 'var(--font-unbounded)', fontSize: 'clamp(22px, 3vw, 30px)', lineHeight: 1.15, fontWeight: 600, letterSpacing: 0 }}>
            {post.title}
          </h2>
          <p style={{ margin: 0, color: 'var(--muted)', lineHeight: 1.55, fontSize: 15 }}>{post.excerpt}</p>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginTop: 6, alignItems: 'center', color: 'var(--muted)', fontSize: 12 }}>
            <span>{post.author}</span>
            <span style={{ color: 'var(--accent-strong)', borderBottom: '1px dashed rgba(29,158,117,0.45)', fontWeight: 600 }}>{t.readArrowLabel}</span>
          </div>
        </div>
      </article>
    </Link>
  )
}

function PostCard({ post }: { post: BlogPostContent }) {
  const t = useT(blogText)
  return (
    <Link href={`/blog/${encodeURIComponent(post.slug)}`} style={cardLink}>
      <article style={{ height: '100%', display: 'flex', flexDirection: 'column', border: '1px solid var(--line)', borderRadius: 8, background: 'var(--card)', overflow: 'hidden' }}>
        <Cover post={post} minHeight={140} label="cover" />
        <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 8, flex: 1 }}>
          <Meta post={post} />
          <h3 style={{ margin: 0, fontFamily: 'var(--font-unbounded)', fontSize: 18, lineHeight: 1.22, fontWeight: 600, letterSpacing: 0 }}>
            {post.title}
          </h3>
          <p style={{ margin: 0, color: 'var(--muted)', lineHeight: 1.48, fontSize: 13, flex: 1 }}>{post.excerpt}</p>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginTop: 4, alignItems: 'center', color: 'var(--muted)', fontSize: 11 }}>
            <span>{post.author}</span>
            <span style={{ color: 'var(--accent-strong)', borderBottom: '1px dashed rgba(29,158,117,0.45)', fontWeight: 600 }}>{t.readLabel}</span>
          </div>
        </div>
      </article>
    </Link>
  )
}

function Meta({ post, tone = 'neutral' }: { post: BlogPostContent; tone?: 'neutral' | 'strong' }) {
  const t = useT(blogText)
  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
      <span style={{ borderRadius: 999, padding: '4px 8px', background: tone === 'strong' ? 'var(--soft)' : 'rgba(var(--card-rgb), 0.92)', border: '1px solid var(--line)', color: tone === 'strong' ? 'var(--accent-strong)' : 'var(--muted)', fontSize: 11, fontWeight: 600 }}>
        {post.tag}
      </span>
      <span style={{ fontSize: 11, color: 'var(--muted)' }}>{post.date} · {post.readMinutes || t.defaultReadMinutes}</span>
    </div>
  )
}

function Cover({ post, minHeight, label }: { post: BlogPostContent; minHeight: number; label: string }) {
  const cover = blogPostCoverUrl(post)
  if (cover) {
    return (
      <div style={{ minHeight, borderBottom: '1px solid var(--line)', background: 'var(--soft)' }}>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={cover} alt="" style={{ display: 'block', width: '100%', height: '100%', minHeight, objectFit: 'cover' }} />
      </div>
    )
  }
  return (
    <div style={{ minHeight, borderBottom: '1px solid var(--line)', background: 'repeating-linear-gradient(135deg, var(--soft), var(--soft) 18px, rgba(29,158,117,0.08) 18px, rgba(29,158,117,0.08) 36px)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--muted)', fontSize: 11, fontFamily: 'ui-monospace, monospace', letterSpacing: '0.05em' }}>
      {label}
    </div>
  )
}

function SubscribeStrip({ title, text, buttonLabel }: { title: string; text: string; buttonLabel: string }) {
  return (
    <section style={{ marginTop: 28, padding: 22, display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) auto', gap: 14, alignItems: 'center', border: '1px solid var(--line)', borderRadius: 8, background: 'var(--soft)' }} className="ms-blog-subscribe">
      <div>
        <h2 style={{ margin: 0, fontFamily: 'var(--font-unbounded)', fontSize: 22, lineHeight: 1.2, letterSpacing: 0 }}>{title}</h2>
        <p style={{ margin: '7px 0 0', color: 'var(--muted)', fontSize: 13, lineHeight: 1.5 }}>{text}</p>
      </div>
      <Link href="/login?next=/chat" style={{ ...navButton, height: 42, background: 'var(--accent)', color: '#fff', borderColor: 'var(--accent)' }}>{buttonLabel}</Link>
    </section>
  )
}

function ClosedSection({ title, text }: { title: string; text: string }) {
  return (
    <main style={{ minHeight: '100dvh', background: 'var(--background)', color: 'var(--foreground)', fontFamily: 'var(--font-golos)' }}>
      <StorefrontHeader active="blog" />
      <section style={{ maxWidth: 720, margin: '0 auto', padding: '80px 24px' }}>
        <h1 style={{ margin: 0, fontFamily: 'var(--font-unbounded)', fontSize: 34, lineHeight: 1.15, letterSpacing: 0 }}>{title}</h1>
        <p style={{ color: 'var(--muted)', lineHeight: 1.6 }}>{text}</p>
      </section>
    </main>
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
} satisfies CSSProperties

const tagButton = {
  border: '1px solid var(--line)',
  borderRadius: 999,
  padding: '7px 12px',
  background: 'var(--card)',
  color: 'var(--muted)',
  fontSize: 13,
  fontFamily: 'inherit',
  cursor: 'pointer',
} satisfies CSSProperties

const activeTagButton = {
  ...tagButton,
  background: 'var(--accent)',
  borderColor: 'var(--accent)',
  color: '#fff',
} satisfies CSSProperties

const pageButton = {
  minWidth: 36,
  height: 36,
  borderRadius: 8,
  border: '1px solid var(--line)',
  background: 'var(--card)',
  color: 'var(--foreground)',
  fontSize: 13,
  fontFamily: 'inherit',
  cursor: 'pointer',
} satisfies CSSProperties

const activePageButton = {
  ...pageButton,
  background: 'var(--accent)',
  borderColor: 'var(--accent)',
  color: '#fff',
} satisfies CSSProperties

const cardLink = {
  display: 'block',
  color: 'inherit',
  textDecoration: 'none',
} satisfies CSSProperties
