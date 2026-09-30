import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { daysLeft } from './_utils'

describe('profile utils P0', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-31T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns empty suffix when activeTo is absent or invalid', () => {
    expect(daysLeft()).toBe('')
    expect(daysLeft(null)).toBe('')
    expect(daysLeft('not-a-date')).toBe('')
  })

  it('ceil-rounds future access to days for active mode chips', () => {
    expect(daysLeft('2026-06-01T11:59:00Z')).toBe(' · осталось 1 дн.')
    expect(daysLeft('2026-06-02T12:01:00Z')).toBe(' · осталось 3 дн.')
  })

  it('never shows negative days for expired access', () => {
    expect(daysLeft('2026-05-30T12:00:00Z')).toBe(' · осталось 0 дн.')
  })
})
