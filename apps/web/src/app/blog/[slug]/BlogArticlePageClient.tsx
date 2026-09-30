'use client'

import type { CSSProperties } from 'react'
import Link from 'next/link'
import { EditableLegalBody } from '@/components/EditableLegalBody'
import { StorefrontHeader } from '@/components/StorefrontHeader'
import { blogPostCoverUrl, type BlogPostContent, getPublishedBlogPosts } from '@/lib/blogContent'
import { getBoolean, getString } from '@/lib/siteContent'
import { useSiteContent } from '@/lib/useSiteContent'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { blogText } from '../translations'

export function BlogArticlePageClient({ slug }: { slug: string }) {
  const content = useSiteContent()
  const locale = useLocale()
  const t = useT(blogText)
  const enabled = getBoolean(content, 'features.blog_enabled', false)
  const post = getPublishedBlogPosts(content, locale).find((item) => item.slug === slug)

  if (!enabled) return <MessagePage title={locale === 'en' ? t.closed.title : getString(content, 'blog.closed.title', 'Блог пока закрыт')} text={locale === 'en' ? t.closed.text : getString(content, 'blog.closed.text', 'Раздел скоро откроется.')} />
  if (!post) return <MessagePage title={locale === 'en' ? t.article.notFoundTitle : getString(content, 'blog.article.not_found_title', 'Статья не найдена')} text={locale === 'en' ? t.article.notFoundText : getString(content, 'blog.article.not_found_text', 'Она могла быть снята с публикации или ещё не опубликована.')} />

  return (
    <main style={{ minHeight: '100dvh', background: 'var(--background)', color: 'var(--foreground)', fontFamily: 'var(--font-golos)' }}>
      <StorefrontHeader active="blog" />
      <article style={{ maxWidth: 760, margin: '0 auto', padding: '42px 24px 72px' }}>
        <Link href="/blog" style={{ color: 'var(--accent-strong)', fontSize: 13, fontWeight: 600, textDecoration: 'none' }}>
          ← {locale === 'en' ? t.article.backLabel : getString(content, 'blog.article.back_label', 'Все статьи')}
        </Link>

        <div style={{ marginTop: 16, display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <span style={tagBadge}>{post.tag}</span>
          <span style={{ color: 'var(--muted)', fontSize: 12 }}>{post.date} · {post.readMinutes || t.defaultReadMinutes} {t.article.readingSuffix}</span>
        </div>

        <h1 style={{ margin: '14px 0 0', fontFamily: 'var(--font-unbounded)', fontWeight: 600, fontSize: 'clamp(28px, 5vw, 42px)', lineHeight: 1.08, letterSpacing: 0 }}>
          {post.title}
        </h1>
        <div style={{ marginTop: 9, color: 'var(--muted)', fontSize: 13 }}>{post.author}</div>

        <Cover post={post} />

        <div style={{ marginTop: 24, fontSize: 15, lineHeight: 1.7 }}>
          <EditableLegalBody body={post.body || post.excerpt} />
        </div>

        <section style={{ marginTop: 30, padding: 18, border: '1px solid var(--line)', borderRadius: 8, background: 'var(--soft)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
          <div style={{ color: 'var(--muted)', fontSize: 13, lineHeight: 1.5 }}>
            {locale === 'en' ? t.article.ctaText : getString(content, 'blog.article.cta_text', 'Можно сразу попробовать режимы на своём вопросе.')}
          </div>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <Link href="/chat" style={{ ...buttonLink, background: 'var(--accent)', borderColor: 'var(--accent)', color: '#fff' }}>{locale === 'en' ? t.article.chatLabel : getString(content, 'blog.article.chat_label', 'Открыть чат')}</Link>
            <Link href="/login?next=/chat" style={buttonLink}>{locale === 'en' ? t.article.loginLabel : getString(content, 'blog.article.login_label', 'Войти')}</Link>
          </div>
        </section>
      </article>
    </main>
  )
}

function MessagePage({ title, text }: { title: string; text: string }) {
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

function Cover({ post }: { post: BlogPostContent }) {
  const cover = blogPostCoverUrl(post)
  if (cover) {
    return (
      <div style={{ marginTop: 20, minHeight: 280, border: '1px solid var(--line)', borderRadius: 8, overflow: 'hidden', background: 'var(--soft)' }}>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={cover} alt="" style={{ display: 'block', width: '100%', minHeight: 280, maxHeight: 420, objectFit: 'cover' }} />
      </div>
    )
  }
  return (
    <div style={{ marginTop: 20, minHeight: 280, border: '1px solid var(--line)', borderRadius: 8, background: 'repeating-linear-gradient(135deg, var(--soft), var(--soft) 18px, rgba(29,158,117,0.08) 18px, rgba(29,158,117,0.08) 36px)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--muted)', fontSize: 11, fontFamily: 'ui-monospace, monospace', letterSpacing: '0.05em' }}>
      article cover
    </div>
  )
}

const tagBadge = {
  borderRadius: 999,
  padding: '4px 8px',
  background: 'var(--soft)',
  border: '1px solid var(--line)',
  color: 'var(--accent-strong)',
  fontSize: 11,
  fontWeight: 600,
} satisfies CSSProperties

const buttonLink = {
  height: 36,
  padding: '0 14px',
  borderRadius: 8,
  border: '1px solid var(--line)',
  background: 'var(--card)',
  color: 'var(--foreground)',
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  fontSize: 13,
  fontWeight: 500,
  textDecoration: 'none',
} satisfies CSSProperties
