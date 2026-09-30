import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'

const apiFetchMock = vi.hoisted(() => vi.fn())
const replaceMock = vi.hoisted(() => vi.fn())
const pushMock = vi.hoisted(() => vi.fn())

vi.mock('@/lib/api', () => ({
  apiFetch: apiFetchMock,
}))

vi.mock('@/components/BrandLogo', () => ({
  BrandLogo: () => <a href="/" aria-label="СТРАТУМ Mindstrata">logo</a>,
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: replaceMock, push: pushMock }),
}))

import ProfilePage from './page'

const profilePayload = {
  ok: true,
  user: { id: 7, email: 'user@test.local', role: 'admin', status: 'active' },
  hasAccess: true,
  allowMessageAnonymization: true,
  activeTo: '2026-06-10T12:00:00Z',
  activeModes: [{ modeId: 10, modeName: 'Психолог', activeTo: '2026-06-10T12:00:00Z' }],
  stats: { dialogs: 3, messages: 12, liveMessages: 4, tokens: 12345 },
  billing: {
    subscription: {
      id: 3,
      status: 'active',
      activeTo: '2026-07-10T12:00:00Z',
      tariffName: 'Профи',
      isRecurring: true,
      autoRenewEnabled: true,
      consecutiveFailures: 0,
      paymentMethodType: 'bank_card',
      paymentMethodStatus: 'active',
    },
    payments: [
      {
        id: 11,
        paymentId: 'yk-profile',
        status: 'paid',
        tariffName: 'Профи',
        amount: '100.00',
        currency: 'RUB',
        subscriptionMonths: 1,
        isRecurring: true,
        autoRenewRequested: true,
        renewalAttempt: 0,
        createdAt: '2026-06-05T12:00:00Z',
      },
    ],
  },
  notifications: {
    contacts: [{ channel: 'email', address: 'user@test.local', verified: true, source: 'yandex_id' }],
    consents: [{ channel: 'in_site', consentType: 'service', status: 'granted', reason: 'Служебные уведомления внутри сервиса' }],
    inbox: [{
      id: 91,
      templateKey: 'service.access_granted',
      consentType: 'service',
      title: 'Доступ включён',
      body: 'Можно вернуться в чат и продолжить работу.',
      status: 'unread',
      createdAt: '2026-06-05T12:00:00Z',
    }],
    reachability: {
      emailAvailable: true,
      phoneVerified: false,
      pushEnabled: false,
      serviceChannels: ['in_site'],
      marketingChannels: [],
      bestChannel: 'in_site',
    },
  },
}

describe('ProfilePage P0', () => {
  beforeEach(() => {
    apiFetchMock.mockReset()
    replaceMock.mockReset()
    pushMock.mockReset()
    localStorage.clear()
    Object.defineProperty(window, 'innerWidth', { value: 1024, writable: true, configurable: true })
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('loads profile, renders access/status cards and admin navigation', async () => {
    apiFetchMock.mockResolvedValue(profilePayload)

    render(<ProfilePage />)

    expect(screen.getByText(/загрузка профиля/i)).toBeTruthy()
    await waitFor(() => expect(screen.getAllByText('user@test.local').length).toBeGreaterThan(0))
    expect(apiFetchMock).toHaveBeenCalledWith('/api/profile')
    expect(screen.getByText('Доступ активен')).toBeTruthy()
    expect(screen.getByText('Психолог')).toBeTruthy()
    expect(screen.getByText('Подписка и платежи')).toBeTruthy()
    expect(screen.getByText(/Профи · active/)).toBeTruthy()
    expect(screen.getByText('bank_card · active')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Отключить автопродление' })).toBeTruthy()
    expect(screen.getByText('Что присылать')).toBeTruthy()
    expect(screen.getByText('Уведомления в Max')).toBeTruthy()
    expect(screen.getByText('Последние пуши')).toBeTruthy()
    expect(screen.getByText('Доступ включён')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Админ' }).getAttribute('href')).toBe('/admin')
    expect(screen.getByRole('link', { name: 'В чат' }).getAttribute('href')).toBe('/chat')
  })

  it('redirects unauthenticated users to /login without showing stale profile state', async () => {
    apiFetchMock.mockRejectedValue(new Error('auth required'))

    render(<ProfilePage />)

    await waitFor(() => expect(replaceMock).toHaveBeenCalledWith('/login'))
    expect(screen.queryByText('Доступ активен')).toBeNull()
  })

  it('shows non-auth API errors inline for support diagnostics', async () => {
    apiFetchMock.mockRejectedValue(new Error('backend unavailable'))

    render(<ProfilePage />)

    await waitFor(() => expect(screen.getByText('backend unavailable')).toBeTruthy())
    expect(replaceMock).not.toHaveBeenCalled()
  })

  // On mount the profile calls several endpoints (/api/profile and others),
  // so mocks are built by path, not by call order.
  function mockProfileApi(
    profile: Record<string, unknown> = profilePayload,
    handlers: Record<string, () => unknown> = {},
  ) {
    apiFetchMock.mockImplementation((path: string) => {
      if (handlers[path]) return Promise.resolve(handlers[path]())
      if (path === '/api/profile') return Promise.resolve(profile)
      return Promise.resolve({ ok: true })
    })
  }

  it('logs out via API and navigates to login even if backend logout response is empty', async () => {
    mockProfileApi(profilePayload, { '/api/auth/logout': () => ({}) })

    render(<ProfilePage />)
    await waitFor(() => expect(screen.getAllByText('user@test.local').length).toBeGreaterThan(0))
    fireEvent.click(screen.getByRole('button', { name: 'Выйти' }))

    await waitFor(() => expect(apiFetchMock).toHaveBeenCalledWith('/api/auth/logout', { method: 'POST' }))
    expect(pushMock).toHaveBeenCalledWith('/login')
  })


  it('renders mobile profile primary actions without bottom navigation', async () => {
    Object.defineProperty(window, 'innerWidth', { value: 390, writable: true, configurable: true })
    apiFetchMock.mockResolvedValue(profilePayload)

    const { container } = render(<ProfilePage />)

    await waitFor(() => expect(screen.getAllByText('user@test.local').length).toBeGreaterThan(0))
    expect(screen.getByRole('link', { name: 'В чат' }).getAttribute('href')).toBe('/chat')
    expect(screen.getByRole('link', { name: 'Админ' }).getAttribute('href')).toBe('/admin')
    expect(screen.getByRole('button', { name: 'Выйти' })).toBeTruthy()
    expect(container.querySelector('.ms-mobile-bottom-nav')).toBeNull()
  })
  it('updates message anonymization setting optimistically', async () => {
    mockProfileApi(profilePayload, {
      '/api/profile/settings': () => ({ ok: true, allowMessageAnonymization: false }),
    })

    render(<ProfilePage />)

    const toggle = await screen.findByRole('switch', { name: 'Убирать личные данные из старой переписки' })
    expect(toggle.getAttribute('aria-checked')).toBe('true')
    expect(screen.getByText(/имена в старых сообщениях будут заменены/i)).toBeTruthy()
    fireEvent.click(toggle)

    await waitFor(() => expect(apiFetchMock).toHaveBeenCalledWith('/api/profile/settings', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ allowMessageAnonymization: false }),
    }))
    expect(toggle.getAttribute('aria-checked')).toBe('false')
    expect(screen.getByText(/обращения будут точнее/i)).toBeTruthy()
  })

  it('shows Max Messenger toggle in "Что присылать" section when not linked', async () => {
    mockProfileApi({
      ...profilePayload,
      maxLinked: false,
      maxBotLink: 'https://max.ru/testbot?start=123',
    })

    render(<ProfilePage />)

    await waitFor(() => expect(screen.getByText('Уведомления в Max')).toBeTruthy())
    // a click on the toggle leads to the bot link (wrapper link)
    const link = screen.getByRole('link', { name: 'Подключить уведомления в Max' })
    expect(link.getAttribute('href')).toContain('max.ru/testbot?start=')
  })

  it('disables autorenew from billing card and reloads profile', async () => {
    let autorenewDisabled = false
    apiFetchMock.mockImplementation((path: string) => {
      if (path === '/api/billing/autorenew/disable') {
        autorenewDisabled = true
        return Promise.resolve({ ok: true, disabled: 1 })
      }
      if (path === '/api/profile') {
        return Promise.resolve(autorenewDisabled
          ? {
              ...profilePayload,
              billing: {
                ...profilePayload.billing,
                subscription: { ...profilePayload.billing.subscription, autoRenewEnabled: false },
              },
            }
          : profilePayload)
      }
      return Promise.resolve({ ok: true })
    })

    render(<ProfilePage />)

    const button = await screen.findByRole('button', { name: 'Отключить автопродление' })
    fireEvent.click(button)

    await waitFor(() => expect(apiFetchMock).toHaveBeenCalledWith('/api/billing/autorenew/disable', { method: 'POST' }))
    await waitFor(() => expect(screen.getByText('выключено')).toBeTruthy())
  })


  it('renders inactive access and missing stats safely', async () => {
    apiFetchMock.mockResolvedValue({
      ok: true,
      user: { id: 8, email: 'plain@test.local', role: 'user', status: 'active' },
      hasAccess: false,
      allowMessageAnonymization: false,
      activeModes: [],
    })

    render(<ProfilePage />)

    await waitFor(() => expect(screen.getByText('Нет активного доступа')).toBeTruthy())
    expect(screen.getByText('Нет активных режимов')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Ввести промокод' }).getAttribute('href')).toBe('/access?force=promo')
    expect(screen.getAllByText('0').length).toBeGreaterThan(0)
  })

  it('does not render removed mobile bottom navigation', async () => {
    Object.defineProperty(window, 'innerWidth', { value: 390, writable: true, configurable: true })
    apiFetchMock.mockResolvedValue(profilePayload)

    const { container } = render(<ProfilePage />)

    await waitFor(() => expect(screen.getAllByText('user@test.local').length).toBeGreaterThan(0))
    expect(container.querySelector('.ms-mobile-bottom-nav')).toBeNull()
    expect(screen.queryByRole('link', { name: /История/i })).toBeNull()
    expect(screen.queryByRole('link', { name: /База/i })).toBeNull()
  })

})
