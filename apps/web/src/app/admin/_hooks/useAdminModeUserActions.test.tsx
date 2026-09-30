import { describe, it, expect, vi, afterEach } from 'vitest'
import type { FormEvent } from 'react'
import { act, renderHook } from '@testing-library/react'

const apiFetchMock = vi.fn()
vi.mock('@/lib/api', () => ({
  apiFetch: (...args: unknown[]) => apiFetchMock(...args),
}))

import { useAdminModeUserActions } from './useAdminModeUserActions'

function makeCtx(overrides: Record<string, unknown> = {}) {
  return {
    created: { email: 'new@test.local', password: 'ignored-old-value', role: 'user', status: 'active', days: '45', modeIds: [10, 20] },
    setCreated: vi.fn(),
    grant: { userId: '', email: '', days: '30', tariffId: '', modeIds: [], dailyMessageLimit: '50' },
    setGrant: vi.fn(),
    tariffForm: { modeIds: [] },
    setTariffForm: vi.fn(),
    promoForm: { targetIds: [] },
    setPromoForm: vi.fn(),
    grantUserDetail: null,
    setGrantUserDetail: vi.fn(),
    exportModes: [],
    modes: [],
    setError: vi.fn(),
    showNotice: vi.fn(),
    loadUsers: vi.fn().mockResolvedValue(undefined),
    handleError: vi.fn(),
    setUserDetail: vi.fn(),
    ...overrides,
  } as any
}

describe('useAdminModeUserActions createUser', () => {
  afterEach(() => {
    apiFetchMock.mockReset()
    vi.restoreAllMocks()
  })

  it('submits create-user payload with empty password and numeric days', async () => {
    apiFetchMock.mockResolvedValue({ ok: true })
    const ctx = makeCtx()
    const { result } = renderHook(() => useAdminModeUserActions(ctx))
    const preventDefault = vi.fn()

    await act(async () => {
      await result.current.createUser({ preventDefault } as unknown as FormEvent)
    })

    expect(preventDefault).toHaveBeenCalled()
    expect(apiFetchMock).toHaveBeenCalledWith('/api/admin/users', {
      method: 'POST',
      body: JSON.stringify({
        email: 'new@test.local',
        password: '',
        role: 'user',
        status: 'active',
        days: 45,
        modeIds: [10, 20],
      }),
    })
  })

  it('resets create-user form and reloads first users page after success', async () => {
    apiFetchMock.mockResolvedValue({ ok: true })
    const ctx = makeCtx()
    const { result } = renderHook(() => useAdminModeUserActions(ctx))

    await act(async () => {
      await result.current.createUser({ preventDefault: vi.fn() } as unknown as FormEvent)
    })

    expect(ctx.showNotice).toHaveBeenCalledWith('Пользователь создан.')
    expect(ctx.setCreated).toHaveBeenCalledWith({ email: '', password: '', role: 'user', status: 'active', days: '30', modeIds: [] })
    expect(ctx.loadUsers).toHaveBeenCalledWith(0)
  })

  it('keeps tariff first mode inside selected tariff modes', () => {
    const ctx = makeCtx({
      tariffForm: { modeIds: [10], firstModeId: 10 },
    })
    const { result } = renderHook(() => useAdminModeUserActions(ctx))

    act(() => {
      result.current.setModeSelection('tariff', [20, 30])
    })

    const update = ctx.setTariffForm.mock.calls[0][0]
    expect(update({ modeIds: [10], firstModeId: 10 })).toEqual({
      modeIds: [20, 30],
      firstModeId: 20,
    })
  })
})
