import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/components/BrandLogo', () => ({
  BrandLogo: () => <a href="/" aria-label="СТРАТУМ Mindstrata">logo</a>,
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

let content = vi.hoisted(() => ({
  'features.blog_enabled': true,
  'blog.posts': [
    {
      slug: 'published-case',
      title: 'Опубликованная статья',
      excerpt: 'Короткое описание',
      tag: 'Кейсы',
      author: 'Редакция',
      date: '2026-06-05',
      body: '# Вводный блок\n\n- первый пункт\n- второй пункт\n\n| Метрика | Значение |\n| --- | --- |\n| Конверсия | выше |\n\n[Внутренняя ссылка](/chat)',
      status: 'published',
      readMinutes: '6 мин',
    },
    {
      slug: 'draft-case',
      title: 'Черновик',
      excerpt: 'Не показывать',
      tag: 'Черновики',
      author: 'Редакция',
      date: '2026-06-05',
      body: 'Скрытый текст',
      status: 'draft',
    },
    {
      slug: 'scheduled-case',
      title: 'Запланировано',
      excerpt: 'Не показывать',
      tag: 'План',
      author: 'Редакция',
      date: '2026-07-01',
      body: 'Скрытый текст',
      status: 'scheduled',
    },
  ],
}))

vi.mock('@/lib/useSiteContent', () => ({
  useSiteContent: () => content,
}))

import { BlogArticlePageClient } from './BlogArticlePageClient'

describe('BlogArticlePage', () => {
  afterEach(() => {
    cleanup()
    content = {
      'features.blog_enabled': true,
      'blog.posts': [
        {
          slug: 'published-case',
          title: 'Опубликованная статья',
          excerpt: 'Короткое описание',
          tag: 'Кейсы',
          author: 'Редакция',
          date: '2026-06-05',
          body: '# Вводный блок\n\n- первый пункт\n- второй пункт\n\n| Метрика | Значение |\n| --- | --- |\n| Конверсия | выше |\n\n[Внутренняя ссылка](/chat)',
          status: 'published',
          readMinutes: '6 мин',
        },
      ],
    }
  })

  it('renders a published article with GFM markdown and CTA links', () => {
    render(<BlogArticlePageClient slug="published-case" />)

    expect(screen.getByRole('heading', { name: 'Опубликованная статья' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Вводный блок' })).toBeTruthy()
    expect(screen.getByText('первый пункт')).toBeTruthy()
    expect(screen.getByRole('table')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Внутренняя ссылка' }).getAttribute('href')).toBe('/chat')
    expect(screen.getByRole('link', { name: 'Открыть чат' }).getAttribute('href')).toBe('/chat')
    expect(screen.getAllByRole('link', { name: 'Войти' }).some((link) => link.getAttribute('href') === '/login?next=/chat')).toBe(true)
  })

  it('does not render draft or scheduled posts by slug', () => {
    render(<BlogArticlePageClient slug="draft-case" />)

    expect(screen.getByRole('heading', { name: 'Статья не найдена' })).toBeTruthy()
    expect(screen.queryByText('Черновик')).toBeNull()
  })

  it('shows closed state when the blog flag is disabled', () => {
    content = { 'features.blog_enabled': false, 'blog.posts': [] }

    render(<BlogArticlePageClient slug="published-case" />)

    expect(screen.getByRole('heading', { name: 'Блог пока закрыт' })).toBeTruthy()
  })
})
