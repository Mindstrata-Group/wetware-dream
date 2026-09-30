import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, act, waitFor } from '@testing-library/react'

// Mock useSiteContent so the CMS content arrives synchronously with the {{vars}} substituted.
const useSiteContentMock = vi.fn<[], Record<string, unknown>>()
vi.mock('@/lib/useSiteContent', () => ({
  useSiteContent: () => useSiteContentMock(),
}))

import AboutPage from './page'

describe('AboutPage — {{vars}} подстановка', () => {
  beforeEach(() => {
    // global fetch for /api/public/demo-modes
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ modes: Array.from({ length: 11 }).map((_, i) => ({ id: i + 1 })) }),
    }) as unknown as typeof fetch
  })
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    useSiteContentMock.mockReset()
  })

  it.skip('заменяет {{modes_count}} в about.intro.title на актуальное число', async () => {
    useSiteContentMock.mockReturnValue({
      'about.intro.title': 'Сервис с {{modes_count}} режимами',
    })
    await act(async () => {
      render(<AboutPage />)
    })
    await waitFor(() => {
      expect(screen.getByText(/11 режимами/i)).toBeTruthy()
    })
  })

  it('подставляет {{paid_count}} и {{total_count}} отдельно', async () => {
    useSiteContentMock.mockReturnValue({
      'about.intro.subtitle': 'Платных {{paid_count}}, всего {{total_count}}',
    })
    await act(async () => {
      render(<AboutPage />)
    })
    await waitFor(() => {
      // fetch returns 11 modes; about/page.tsx maps "paid=total=length"
      expect(screen.getByText(/Платных 11, всего 11/)).toBeTruthy()
    })
  })

  it('игнорирует неизвестные плейсхолдеры (оставляет как есть)', async () => {
    useSiteContentMock.mockReturnValue({
      'about.comparison.title': 'Сравнение {{unknown_var}}',
    })
    await act(async () => {
      render(<AboutPage />)
    })
    await waitFor(() => {
      expect(screen.getByText(/Сравнение \{\{unknown_var\}\}/)).toBeTruthy()
    })
  })

  it('CMS-значения отображаются для счётчика и заголовков таблицы', async () => {
    useSiteContentMock.mockReturnValue({
      'about.stats.modes_label': 'модусов',
      'about.comparison.header_aspect': 'Параметр',
      'about.comparison.header_stratum': 'НАШИ',
    })
    await act(async () => {
      render(<AboutPage />)
    })
    await waitFor(() => {
      expect(screen.getByText('модусов')).toBeTruthy()
      expect(screen.getByText('Параметр')).toBeTruthy()
      expect(screen.getByText('НАШИ')).toBeTruthy()
    })
  })

  it('кейс-инсенситив: {{Modes_Count}} тоже работает', async () => {
    useSiteContentMock.mockReturnValue({
      'about.intro.title': 'Заголовок {{Modes_Count}}',
    })
    await act(async () => {
      render(<AboutPage />)
    })
    await waitFor(() => {
      expect(screen.getByText(/Заголовок 11/)).toBeTruthy()
    })
  })

  it('fallback дефолт когда CMS не отдаёт ключ', async () => {
    useSiteContentMock.mockReturnValue({})
    await act(async () => {
      render(<AboutPage />)
    })
    // Defaults from the fallback (see about/page.tsx):
    await waitFor(() => {
      expect(screen.getByText(/Не просто нейросеть/i)).toBeTruthy()
    })
  })
})
