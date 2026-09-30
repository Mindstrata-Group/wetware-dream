import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
let cmsContent = vi.hoisted(() => ({} as Record<string, unknown>))

vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/useSiteContent', () => ({
  useSiteContent: () => cmsContent,
}))
vi.mock('@/components/BrandLogo', () => ({
  BrandLogo: () => <a href="/" aria-label="СТРАТУМ Mindstrata">logo</a>,
}))
vi.mock('@/components/CookieBanner', () => ({
  CookieBanner: () => null,
}))
vi.mock('./_home/DotsBackground', () => ({
  DotsBackground: () => null,
}))
vi.mock('./_home/ModeStrip', () => ({
  ModeStrip: () => null,
}))
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

import HomePage from './page'

describe('HomePage feature visibility', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', { value: 1280, writable: true, configurable: true })
    apiFetch.mockRejectedValue(new Error('guest'))
    globalThis.fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, modes: [] }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as unknown as typeof fetch
    cmsContent = {}
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('does not show blog or shop in the top navigation when runtime flags are off', async () => {
    render(<HomePage />)

    await waitFor(() => expect(apiFetch).toHaveBeenCalledWith('/api/profile'))
    expect(screen.queryByRole('link', { name: 'Блог' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Магазин решений' })).toBeNull()
  })

  it('shows blog and shop in the top navigation when runtime flags are on', async () => {
    cmsContent = {
      'features.blog_enabled': true,
      'features.shop_enabled': true,
      'landing.blog_button_label': 'Журнал',
      'landing.shop_button_label': 'Тарифы',
    }

    render(<HomePage />)

    await waitFor(() => expect(screen.getByRole('link', { name: 'Журнал' }).getAttribute('href')).toBe('/blog'))
    expect(screen.getByRole('link', { name: 'Тарифы' }).getAttribute('href')).toBe('/shop')
  })

  it('normalizes legacy hero copy into auto mode switching value', async () => {
    cmsContent = {
      'landing.hero.title': 'Сам поймёт — включит нужный режим',
      'landing.hero.subtitle': 'Внутри много режимов для текста, решений, переговоров и сложных ситуаций. Пишите обычным языком: система поймёт задачу, переключит режим и продолжит разбор.',
    }

    render(<HomePage />)

    await waitFor(() => expect(screen.getByRole('heading', { name: 'Опишите задачу — Стратум сам выберет режим' })).toBeTruthy())
    expect(screen.getByText(/Внутри 11 режимов/)).toBeTruthy()
    expect(screen.getByText(/Стратум поймёт задачу, переключит режим и продолжит разбор/)).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'Сам поймёт — включит нужный режим' })).toBeNull()
  })
})
