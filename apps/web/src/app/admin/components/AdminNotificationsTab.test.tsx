import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetchMock = vi.hoisted(() => vi.fn())
const useAdminPageContextMock = vi.hoisted(() => vi.fn())

vi.mock('@/lib/api', () => ({
  apiFetch: apiFetchMock,
}))

vi.mock('./AdminPageContext', () => ({
  useAdminPageContext: useAdminPageContextMock,
}))

import { AdminNotificationsTab } from './AdminNotificationsTab'

describe('AdminNotificationsTab', () => {
  beforeEach(() => {
    apiFetchMock.mockReset()
    // History is loaded on mount; empty by default.
    apiFetchMock.mockImplementation((path: string) => {
      if (path === '/api/admin/notifications/history') return Promise.resolve({ ok: true, items: [] })
      return Promise.resolve({ ok: true })
    })
    useAdminPageContextMock.mockReturnValue({ modes: [], promocodes: [] })
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders audience options, channels and send button', () => {
    render(<AdminNotificationsTab />)
    expect(screen.getByText('Все')).toBeTruthy()
    expect(screen.getByText('По промокодам')).toBeTruthy()
    expect(screen.getByText('По режимам')).toBeTruthy()
    expect(screen.queryByText('Сайт (входящие)')).toBeFalsy()
    expect(screen.getByText('Max')).toBeTruthy()
    expect(screen.getByText('Telegram')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Предпросмотр получателей' })).toBeTruthy()
  })

  it('sends notification to all users with title, body and channels', async () => {
    apiFetchMock.mockImplementation((path: string) => {
      if (path === '/api/admin/notifications/history') return Promise.resolve({ ok: true, items: [] })
      if (path === '/api/admin/notifications/send') {
        return Promise.resolve({ ok: true, recipientCount: 10, delivered: { telegram: 8 } })
      }
      return Promise.resolve({ ok: true })
    })

    render(<AdminNotificationsTab />)

    fireEvent.change(screen.getByPlaceholderText('Заголовок уведомления'), { target: { value: 'Тест' } })
    fireEvent.change(screen.getByPlaceholderText(/Текст уведомления/), { target: { value: 'Текст тест' } })
    // Two-step sending: the first click previews the message, the second sends it.
    fireEvent.click(screen.getByRole('button', { name: 'Отправить' }))
    await waitFor(() => expect(screen.getByText(/Так будет выглядеть сообщение/)).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Подтвердить отправку' }))

    await waitFor(() => expect(apiFetchMock).toHaveBeenCalledWith('/api/admin/notifications/send', {
      method: 'POST',
      body: JSON.stringify({
        audience: 'all',
        promocodeIds: [],
        promoFilters: [],
        modeIds: [],
        modeFilter: undefined,
        periodFrom: undefined,
        periodTo: undefined,
        excludeUserIds: [],
        title: 'Тест',
        body: 'Текст тест',
        channels: ['telegram'],
      }),
    }))
    await waitFor(() => expect(screen.getByText(/Отправлено \(10 получателей\)/)).toBeTruthy())
  })

  it('requests recipients preview and shows counts', async () => {
    apiFetchMock.mockImplementation((path: string) => {
      if (path === '/api/admin/notifications/history') return Promise.resolve({ ok: true, items: [] })
      if (path === '/api/admin/notifications/preview') {
        return Promise.resolve({
          ok: true,
          recipientCount: 3,
          emailCount: 2,
          phoneCount: 1,
          maxLinkedCount: 1,
          telegramLinkedCount: 0,
          recipients: [
            { id: 7, email: 'a@b.c', phone: '+7900', maxLinked: true, telegramLinked: false },
          ],
          previewTruncated: false,
        })
      }
      return Promise.resolve({ ok: true })
    })

    render(<AdminNotificationsTab />)
    fireEvent.click(screen.getByRole('button', { name: 'Предпросмотр получателей' }))

    await waitFor(() => expect(screen.getByText('Получателей: 3')).toBeTruthy())
    expect(screen.getByText(/Max: 1/)).toBeTruthy()
    expect(screen.getByText('a@b.c')).toBeTruthy()
  })

  it('shows validation error when title or body is empty', async () => {
    render(<AdminNotificationsTab />)

    fireEvent.click(screen.getByRole('button', { name: 'Отправить' }))

    await waitFor(() => expect(screen.getByText('Заголовок и текст обязательны')).toBeTruthy())
    expect(apiFetchMock).not.toHaveBeenCalledWith('/api/admin/notifications/send', expect.anything())
  })
})
