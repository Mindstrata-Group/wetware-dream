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
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

import { StorefrontHeader } from './StorefrontHeader'

describe('StorefrontHeader feature visibility', () => {
  beforeEach(() => {
    apiFetch.mockRejectedValue(new Error('guest'))
    cmsContent = {}
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('hides blog and shop links by default', async () => {
    render(<StorefrontHeader />)

    await waitFor(() => expect(apiFetch).toHaveBeenCalledWith('/api/profile'))
    expect(screen.queryByRole('link', { name: 'Блог' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Магазин решений' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Войти' }).getAttribute('href')).toBe('/login')
  })

  it('shows only links enabled by runtime CMS flags', () => {
    cmsContent = {
      'features.blog_enabled': true,
      'features.shop_enabled': false,
      'landing.blog_button_label': 'Журнал',
      'landing.shop_button_label': 'Тарифы',
    }

    render(<StorefrontHeader active="blog" />)

    expect(screen.getByRole('link', { name: 'Журнал' }).getAttribute('href')).toBe('/blog')
    expect(screen.queryByRole('link', { name: 'Тарифы' })).toBeNull()
  })
})
