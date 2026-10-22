import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'

let currentLocale = 'ru'

vi.mock('@/lib/i18n/server', () => ({
  getLocale: vi.fn(async () => currentLocale),
  getDict: vi.fn(async (dict: Record<string, unknown>) => dict[currentLocale]),
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

import NotFound, { notFoundTranslations } from './not-found'

describe('NotFound page (/not-found)', () => {
  beforeEach(() => {
    currentLocale = 'ru'
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders default Russian locale with friendly message and navigation buttons', async () => {
    currentLocale = 'ru'
    const ui = await NotFound()
    render(ui)

    const heading = screen.getByRole('heading', { level: 1 })
    expect(heading.textContent).toBe('404')
    expect(screen.getByText('Эта страница не существует или была перемещена')).toBeTruthy()

    const homeLink = screen.getByRole('link', { name: 'На главную' })
    expect(homeLink).toBeTruthy()
    expect(homeLink.getAttribute('href')).toBe('/')

    const chatLink = screen.getByRole('link', { name: 'В чат' })
    expect(chatLink).toBeTruthy()
    expect(chatLink.getAttribute('href')).toBe('/chat')
  })

  it('renders English locale with friendly message and navigation buttons', async () => {
    currentLocale = 'en'
    const ui = await NotFound()
    render(ui)

    const heading = screen.getByRole('heading', { level: 1 })
    expect(heading.textContent).toBe('404')
    expect(screen.getByText('This page does not exist or has moved')).toBeTruthy()

    const homeLink = screen.getByRole('link', { name: 'Home' })
    expect(homeLink).toBeTruthy()
    expect(homeLink.getAttribute('href')).toBe('/')

    const chatLink = screen.getByRole('link', { name: 'Chat' })
    expect(chatLink).toBeTruthy()
    expect(chatLink.getAttribute('href')).toBe('/chat')
  })

  it('contains valid dictionary entries for all supported locales', () => {
    expect(notFoundTranslations.ru).toBeDefined()
    expect(notFoundTranslations.en).toBeDefined()
    expect(notFoundTranslations.ru.title).toBe('404')
    expect(notFoundTranslations.en.title).toBe('404')
    expect(notFoundTranslations.ru.message).toBeTruthy()
    expect(notFoundTranslations.en.message).toBeTruthy()
    expect(notFoundTranslations.ru.homeButton).toBeTruthy()
    expect(notFoundTranslations.en.homeButton).toBeTruthy()
    expect(notFoundTranslations.ru.chatButton).toBeTruthy()
    expect(notFoundTranslations.en.chatButton).toBeTruthy()
  })
})
