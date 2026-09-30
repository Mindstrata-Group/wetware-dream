import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, act, cleanup } from '@testing-library/react'
import { CookieBanner } from './CookieBanner'

const STORAGE_KEY = 'mindstrata_cookie_consent_id'

// Mock apiFetch so /api/cookie-consent does not try to make a real HTTP call.
vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn().mockResolvedValue({ ok: true }),
}))

describe('CookieBanner state machine', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    localStorage.clear()
  })
  afterEach(() => {
    cleanup()
    localStorage.clear()
    vi.useRealTimers()
  })

  it('баннер появляется когда consent НЕ сохранён', async () => {
    render(<CookieBanner />)
    // useEffect -> doShow -> setPhase('showing') synchronously after mount
    expect(screen.getByText('Принять')).toBeTruthy()
    expect(screen.getByRole('region', { name: /cookie/i })).toBeTruthy()
  })

  it('баннер НЕ появляется когда consent сохранён', async () => {
    localStorage.setItem(STORAGE_KEY, 'previous-consent-id')
    render(<CookieBanner />)
    expect(screen.queryByText('Принять')).toBeNull()
  })

  it('клик Принять → сохраняет consent в localStorage', async () => {
    render(<CookieBanner />)
    const btn = screen.getByText('Принять')
    await act(async () => {
      fireEvent.click(btn)
    })
    expect(localStorage.getItem(STORAGE_KEY)).toBeTruthy()
    expect(localStorage.getItem(STORAGE_KEY)?.length).toBeGreaterThan(5)
  })

  it('после Принять баннер скрывается (через Thanos)', async () => {
    render(<CookieBanner />)
    const btn = screen.getByText('Принять')
    await act(async () => {
      fireEvent.click(btn)
      await vi.advanceTimersByTimeAsync(1900) // > THANOS_DURATION_MS (1800)
    })
    expect(screen.queryByText('Принять')).toBeNull()
  })

  it('15 сек без клика → автоматический Thanos dismiss', async () => {
    render(<CookieBanner />)
    expect(screen.getByText('Принять')).toBeTruthy()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000) // SHOW_DURATION_MS
      await vi.advanceTimersByTimeAsync(1900)   // THANOS_DURATION_MS
    })
    expect(screen.queryByText('Принять')).toBeNull()
  })

  it('через 2 мин после dismiss баннер возвращается (cycle)', async () => {
    render(<CookieBanner />)
    expect(screen.getByText('Принять')).toBeTruthy()
    // wait for dismiss
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000)
      await vi.advanceTimersByTimeAsync(1900)
    })
    expect(screen.queryByText('Принять')).toBeNull()
    // wait for REAPPEAR_DELAY_MS (120_000)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(120_000)
    })
    expect(screen.getByText('Принять')).toBeTruthy()
  })

  it('cross-tab: storage event с newValue=consent → баннер скрывается', async () => {
    render(<CookieBanner />)
    expect(screen.getByText('Принять')).toBeTruthy()
    await act(async () => {
      // Accept was clicked in another tab -> the storage event arrived here.
      localStorage.setItem(STORAGE_KEY, 'cross-tab-consent')
      window.dispatchEvent(new StorageEvent('storage', {
        key: STORAGE_KEY,
        newValue: 'cross-tab-consent',
      }))
    })
    expect(screen.queryByText('Принять')).toBeNull()
  })

  it('mindstrata:cookie-consent-cleared event → возрождает баннер', async () => {
    localStorage.setItem(STORAGE_KEY, 'old-id')
    render(<CookieBanner />)
    expect(screen.queryByText('Принять')).toBeNull()
    // Some flow wipes consent (e.g. logout) and emits an event.
    await act(async () => {
      localStorage.removeItem(STORAGE_KEY)
      window.dispatchEvent(new Event('mindstrata:cookie-consent-cleared'))
    })
    expect(screen.getByText('Принять')).toBeTruthy()
  })

  it('Подробнее ссылается на /privacy/cookies', () => {
    render(<CookieBanner />)
    const link = screen.getByRole('link', { name: /подробнее/i })
    expect(link.getAttribute('href')).toBe('/privacy/cookies')
  })
})
