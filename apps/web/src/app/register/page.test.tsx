import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

const redirectToRegisterOauthMock = vi.hoisted(() => vi.fn())

vi.mock('@/components/BrandLogo', () => ({
  BrandLogo: () => <a href="/" aria-label="СТРАТУМ Mindstrata">logo</a>,
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

vi.mock('./_parts/oauth', () => ({
  REGISTER_OAUTH_HREF: '/api/auth/oauth/yandex/start?next=/profile&intent=register',
  redirectToRegisterOauth: redirectToRegisterOauthMock,
}))

import RegisterPage from './page'

describe('RegisterPage P0', () => {
  afterEach(() => {
    cleanup()
    redirectToRegisterOauthMock.mockClear()
  })

  it('renders critical register navigation and promo fallback links', () => {
    render(<RegisterPage />)

    expect(screen.getByRole('heading', { name: 'Регистрация' })).toBeTruthy()
    expect(screen.getByRole('link', { name: /главная/i }).getAttribute('href')).toBe('/')
    expect(screen.getAllByRole('link', { name: /войти/i }).some(link => link.getAttribute('href') === '/login')).toBe(true)
    expect(screen.getByRole('link', { name: /ввести промокод/i }).getAttribute('href')).toBe('/access?force=promo')
    expect(screen.getByRole('link', { name: /политику конфиденциальности/i }).getAttribute('href')).toBe('/privacy')
  })

  it('blocks Yandex OAuth until user accepts legal terms', () => {
    render(<RegisterPage />)

    fireEvent.click(screen.getByRole('button', { name: /зарегистрироваться с яндекс id/i }))

    expect(redirectToRegisterOauthMock).not.toHaveBeenCalled()
    expect(screen.getByRole('alert').textContent).toMatch(/примите политику/i)
    expect(screen.getByRole('checkbox').getAttribute('aria-checked')).toBe('false')
  })

  it('clears legal error and redirects after checkbox is toggled with mouse', () => {
    render(<RegisterPage />)

    fireEvent.click(screen.getByRole('button', { name: /зарегистрироваться с яндекс id/i }))
    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: /зарегистрироваться с яндекс id/i }))

    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByRole('checkbox').getAttribute('aria-checked')).toBe('true')
    expect(redirectToRegisterOauthMock).toHaveBeenCalledTimes(1)
  })

  it('supports keyboard toggling of the legal checkbox', () => {
    render(<RegisterPage />)
    const checkbox = screen.getByRole('checkbox')

    fireEvent.keyDown(checkbox, { key: 'Enter' })
    expect(checkbox.getAttribute('aria-checked')).toBe('true')

    fireEvent.keyDown(checkbox, { key: ' ' })
    expect(checkbox.getAttribute('aria-checked')).toBe('false')
  })
})
