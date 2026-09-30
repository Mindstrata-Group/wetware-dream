import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup, fireEvent } from '@testing-library/react'
import type { ReactNode } from 'react'

const apiFetchMock = vi.fn()
let search = ''

vi.mock('@/lib/api', async (orig) => {
  const actual = await orig<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiFetch: (...args: unknown[]) => apiFetchMock(...args),
  }
})

vi.mock('next/navigation', () => ({
  useSearchParams: () => new URLSearchParams(search),
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: ReactNode }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

import { ApiError } from '@/lib/api'
import PromoAdminPage from './page'

function renderWithUrl(url: string) {
  search = url.split('?')[1] || ''
  return render(<PromoAdminPage />)
}

describe('PromoAdminPage regressions', () => {
  afterEach(() => {
    cleanup()
    apiFetchMock.mockReset()
    vi.restoreAllMocks()
    search = ''
  })

  it('regression_promo_admin_secret_required_2026_06_01: без key в URL — error message', async () => {
    renderWithUrl('/promo-admin?promo=TEST')

    expect(await screen.findByText(/отсутствует параметр key/i)).toBeTruthy()
    expect(apiFetchMock).not.toHaveBeenCalled()
  })

  it('regression_promo_admin_secret_required_2026_06_01: 403 от API — error не падает приложение', async () => {
    apiFetchMock.mockRejectedValueOnce(
      new ApiError('promo admin link is invalid', 403, { error: 'promo admin link is invalid' }),
    )

    renderWithUrl('/promo-admin?promo=TEST&key=invalid')

    expect(await screen.findByText(/не активна или не найдена/i)).toBeTruthy()
    await waitFor(() => {
      expect(screen.queryByText(/Application error/i)).toBeNull()
    })
  })

  it('disables summarization and shows message diagnostics when promo has no messages', async () => {
    apiFetchMock.mockResolvedValueOnce({
      ok: true,
      promo: {
        id: 1,
        code: 'EMPTY',
        usedCount: 2,
        maxUses: 10,
        summaryUsed: 0,
        summaryRemaining: null,
        messageCount: 0,
        activationsWithoutMessages: 2,
      },
      prompts: [{ id: 7, name: 'Default summary', isDefault: true }],
      modes: [{ id: 3, name: 'Demo chat' }],
    })

    renderWithUrl('/promo-admin?promo=EMPTY&key=valid')

    expect(await screen.findByText(/Уже есть сообщений: 0/i)).toBeTruthy()
    expect(screen.getByText(/Активаций без первого сообщения: 2/i)).toBeTruthy()
    expect(screen.getByText(/пока нечего резюмировать/i)).toBeTruthy()

    const summarizeButton = screen.getByRole('button', { name: /Начать резюмирование/i })
    expect((summarizeButton as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(summarizeButton)
    expect(apiFetchMock).toHaveBeenCalledTimes(1)
  })

  it('loads persisted summary history from promo status and opens saved result', async () => {
    apiFetchMock.mockResolvedValueOnce({
      ok: true,
      promo: {
        id: 2,
        code: 'WITHHISTORY',
        usedCount: 1,
        maxUses: 10,
        summaryUsed: 1,
        summaryRemaining: null,
        messageCount: 4,
        activationsWithoutMessages: 0,
      },
      prompts: [{ id: 7, name: 'Default summary', isDefault: true }],
      modes: [{ id: 3, name: 'Demo chat' }],
      history: [{
        ok: true,
        summaryId: 99,
        result: 'Persisted result after refresh',
        messageCount: 4,
        sourceBytes: 120,
        approxTokens: 30,
        used: 1,
        remaining: null,
        createdAt: '2026-06-02T10:00:00Z',
        modeLabel: 'Demo chat',
      }],
    })

    renderWithUrl('/promo-admin?promo=WITHHISTORY&key=valid')

    await screen.findByText(/История резюмирований/i)
    const historyItem = screen.getByRole('button', { name: /4 сообщений/i })
    fireEvent.click(historyItem)

    expect(await screen.findByText(/Persisted result after refresh/i)).toBeTruthy()
  })


  it('prepends a successful summarize result to persisted history controls', async () => {
    apiFetchMock
      .mockResolvedValueOnce({
        ok: true,
        promo: {
          id: 4,
          code: 'SUMOK',
          usedCount: 1,
          maxUses: 10,
          summaryUsed: 0,
          summaryRemaining: 2,
          messageCount: 3,
          activationsWithoutMessages: 0,
        },
        prompts: [{ id: 7, name: 'Default summary', isDefault: true }],
        modes: [{ id: 3, name: 'Demo chat' }],
        history: [],
      })
      .mockResolvedValueOnce({
        ok: true,
        summaryId: 100,
        result: 'Fresh summary result',
        messageCount: 3,
        sourceBytes: 300,
        approxTokens: 75,
        used: 1,
        remaining: 1,
        createdAt: '2026-06-02T11:00:00Z',
      })

    renderWithUrl('/promo-admin?promo=SUMOK&key=valid')

    const summarizeButton = await screen.findByRole('button', { name: /Начать резюмирование/i })
    expect((summarizeButton as HTMLButtonElement).disabled).toBe(false)
    fireEvent.click(summarizeButton)

    await waitFor(() => expect(apiFetchMock).toHaveBeenCalledTimes(2))
    expect(apiFetchMock).toHaveBeenLastCalledWith('/api/promo-admin/summarize', {
      method: 'POST',
      body: JSON.stringify({ code: 'SUMOK', key: 'valid', modeIds: [], promptId: 7 }),
    })
    expect(await screen.findByText(/Fresh summary result/i)).toBeTruthy()
    expect(screen.getByRole('button', { name: /3 сообщений/i })).toBeTruthy()
  })


  it('shows friendly summary-limit error without adding a history item', async () => {
    apiFetchMock
      .mockResolvedValueOnce({
        ok: true,
        promo: {
          id: 5,
          code: 'LIMITED',
          usedCount: 1,
          maxUses: 10,
          summaryUsed: 1,
          summaryRemaining: 0,
          messageCount: 2,
          activationsWithoutMessages: 0,
        },
        prompts: [{ id: 7, name: 'Default summary', isDefault: true }],
        modes: [{ id: 3, name: 'Demo chat' }],
        history: [],
      })
      .mockRejectedValueOnce(new ApiError('summary limit exhausted', 403, { error: 'summary limit exhausted' }))

    renderWithUrl('/promo-admin?promo=LIMITED&key=valid')

    fireEvent.click(await screen.findByRole('button', { name: /Начать резюмирование/i }))

    expect(await screen.findByText(/Лимит резюмирований закончился/i)).toBeTruthy()
    expect(screen.queryByText(/Результат резюмирования/i)).toBeNull()
    expect(screen.queryByRole('button', { name: /2 сообщений/i })).toBeNull()
  })


  it('copies the absolute promo access URL from the QR actions', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    apiFetchMock.mockResolvedValueOnce({
      ok: true,
      promo: {
        id: 6,
        code: 'COPYME',
        usedCount: 1,
        maxUses: 10,
        summaryUsed: 0,
        summaryRemaining: null,
        messageCount: 2,
        activationsWithoutMessages: 0,
      },
      prompts: [{ id: 7, name: 'Default summary', isDefault: true }],
      modes: [{ id: 3, name: 'Demo chat' }],
      history: [],
    })

    renderWithUrl('/promo-admin?promo=COPYME&key=valid')

    fireEvent.click(await screen.findByRole('button', { name: /Скопировать ссылку QR/i }))

    expect(writeText).toHaveBeenCalledWith(new URL('/access?promo=COPYME', window.location.origin).toString())
  })

  it('downloads the visible summary result as a promo markdown file', async () => {
    const createObjectURL = vi.fn(() => 'blob:summary-md')
    const revokeObjectURL = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', { value: createObjectURL, configurable: true })
    Object.defineProperty(URL, 'revokeObjectURL', { value: revokeObjectURL, configurable: true })
    apiFetchMock.mockResolvedValueOnce({
      ok: true,
      promo: {
        id: 7,
        code: 'DOWNME',
        usedCount: 1,
        maxUses: 10,
        summaryUsed: 1,
        summaryRemaining: null,
        messageCount: 5,
        activationsWithoutMessages: 0,
      },
      prompts: [{ id: 7, name: 'Default summary', isDefault: true }],
      modes: [{ id: 3, name: 'Demo chat' }],
      history: [{
        ok: true,
        summaryId: 101,
        result: 'Downloadable persisted summary',
        messageCount: 5,
        sourceBytes: 200,
        approxTokens: 50,
        used: 1,
        remaining: null,
        createdAt: '2026-06-02T12:00:00Z',
        modeLabel: 'Demo chat',
      }],
    })

    renderWithUrl('/promo-admin?promo=DOWNME&key=valid')

    fireEvent.click(await screen.findByRole('button', { name: /5 сообщений/i }))
    const click = vi.fn()
    const anchor = { href: '', download: '', click } as unknown as HTMLAnchorElement
    const originalCreateElement = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tagName: string, options?: ElementCreationOptions) => {
      if (tagName.toLowerCase() === 'a') return anchor
      return originalCreateElement(tagName, options)
    })
    fireEvent.click(await screen.findByRole('button', { name: /Скачать MD/i }))

    expect(createObjectURL).toHaveBeenCalledTimes(1)
    const blob = createObjectURL.mock.calls[0][0] as Blob
    expect(blob.type).toBe('text/markdown;charset=utf-8')
    expect(anchor.href).toBe('blob:summary-md')
    expect(anchor.download).toBe('promo-DOWNME-summary.md')
    expect(click).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:summary-md')
  })

  it('opens activation QR code fullscreen and closes it by the next click', async () => {
    apiFetchMock.mockResolvedValueOnce({
      ok: true,
      promo: {
        id: 3,
        code: 'QRTEST',
        usedCount: 1,
        maxUses: 10,
        summaryUsed: 0,
        summaryRemaining: null,
        messageCount: 1,
        activationsWithoutMessages: 0,
      },
      prompts: [{ id: 7, name: 'Default summary', isDefault: true }],
      modes: [{ id: 3, name: 'Demo chat' }],
      history: [],
    })

    renderWithUrl('/promo-admin?promo=QRTEST&key=valid')

    const qrButton = await screen.findByRole('button', { name: /Увеличить QR-код активации/i })
    fireEvent.click(qrButton)
    const dialog = await screen.findByRole('dialog', { name: /Закрыть QR-код активации/i })
    expect(dialog).toBeTruthy()

    fireEvent.click(dialog)
    expect(screen.queryByRole('dialog', { name: /Закрыть QR-код активации/i })).toBeNull()
  })

})
