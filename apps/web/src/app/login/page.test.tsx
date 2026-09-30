import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

const replaceMock = vi.hoisted(() => vi.fn())
let search = vi.hoisted(() => new URLSearchParams())

vi.mock('@/components/BrandLogo', () => ({
  BrandLogo: () => <a href="/" aria-label="СТРАТУМ Mindstrata">logo</a>,
}))

vi.mock('@/lib/useApi', () => ({
  useApi: () => ({ data: null, loading: false, error: null, refetch: vi.fn() }),
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: replaceMock }),
  useSearchParams: () => search,
}))

import LoginPage from './page'

describe('LoginPage OAuth errors P0', () => {
  beforeEach(() => {
    replaceMock.mockReset()
    search = new URLSearchParams()
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('explains missing Yandex registration and keeps promo return path', () => {
    search = new URLSearchParams('error=oauth_account&next=/access?force=promo')

    render(<LoginPage />)

    expect(screen.getByText(/Этот Яндекс ID ещё не зарегистрирован/i)).toBeTruthy()
    expect(screen.getByRole('link', { name: /Зарегистрироваться с Яндекс ID/i }).getAttribute('href')).toBe('/register?next=%2Faccess%3Fforce%3Dpromo')
  })

  it('shows guest access transfer failure instead of a generic social login error', () => {
    search = new URLSearchParams('error=guest_access')

    render(<LoginPage />)

    expect(screen.getByText(/гостевой промокод и историю не удалось перенести/i)).toBeTruthy()
  })
})
