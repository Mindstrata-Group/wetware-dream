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
  'blog.hero.title': 'Журнал методик',
}))

vi.mock('@/lib/useSiteContent', () => ({
  useSiteContent: () => content,
}))

import BlogPage from './page'

describe('BlogPage', () => {
  afterEach(() => {
    cleanup()
    content = { 'features.blog_enabled': true, 'blog.hero.title': 'Журнал методик' }
  })

  it('renders product articles when blog flag is enabled', () => {
    render(<BlogPage />)

    expect(screen.getByRole('heading', { name: 'Журнал методик' })).toBeTruthy()
    expect(screen.getByText(/QR-упражнение на выступлении/i)).toBeTruthy()
    expect(screen.getByText(/Еженедельный срез ошибок/i)).toBeTruthy()
    expect(screen.getByRole('link', { name: /QR-упражнение на выступлении/i }).getAttribute('href')).toBe('/blog/speaker-qr-exercise')
  })

  it('shows closed state when blog flag is disabled', () => {
    content = { 'features.blog_enabled': false, 'blog.hero.title': 'Журнал методик' }

    render(<BlogPage />)

    expect(screen.getByRole('heading', { name: 'Блог пока закрыт' })).toBeTruthy()
  })

  it('renders admin-managed published posts and hides drafts', () => {
    content = {
      'features.blog_enabled': true,
      'blog.hero.title': 'Журнал методик',
      'blog.posts': [
        {
          slug: 'admin-case',
          title: 'Пост из админки',
          excerpt: 'Текст карточки',
          tag: 'Кейсы',
          author: 'Редакция',
          date: '2026-06-05',
          body: 'Полный текст',
          status: 'published',
          readMinutes: '4 мин',
        },
        {
          slug: 'draft-case',
          title: 'Черновик не виден',
          excerpt: 'Не показывать',
          tag: 'Черновики',
          author: 'Редакция',
          date: '2026-06-05',
          body: 'Полный текст',
          status: 'draft',
        },
      ],
    }

    render(<BlogPage />)

    expect(screen.getByText('Пост из админки')).toBeTruthy()
    expect(screen.getAllByText('Кейсы').length).toBeGreaterThan(0)
    expect(screen.getByText(/4 мин/i)).toBeTruthy()
    expect(screen.getByRole('link', { name: /Пост из админки/i }).getAttribute('href')).toBe('/blog/admin-case')
    expect(screen.queryByText('Черновик не виден')).toBeNull()
  })
})
