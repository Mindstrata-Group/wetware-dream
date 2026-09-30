import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())

vi.mock('@/lib/api', () => ({ apiFetch }))

import { AdminPaymentsTab } from './AdminPaymentsTab'

describe('AdminPaymentsTab', () => {
  afterEach(() => {
    cleanup()
    apiFetch.mockReset()
  })

  it('loads billing lifecycle rows and can run due renewals', async () => {
    apiFetch
      .mockResolvedValueOnce({
        payments: [
          {
            id: 1,
            userId: 7,
            subscriptionId: 11,
            email: 'payer@test.local',
            tariffName: 'Профи',
            paymentId: 'yk-admin',
            status: 'pending',
            amount: '100.00',
            currency: 'RUB',
            isRecurring: true,
            autoRenewRequested: true,
            renewalAttempt: 2,
            createdAt: '2026-06-05T12:00:00Z',
            autoRenewEnabled: true,
            paymentMethodType: 'bank_card',
            paymentMethodStatus: 'active',
          },
        ],
      })
      .mockResolvedValueOnce({
        testModeEnabled: false,
        activeMode: 'live',
        activeConfigured: true,
        liveConfigured: true,
        liveShopIdMasked: '****2345',
        testConfigured: false,
      })
      .mockResolvedValueOnce({ ok: true, processed: 1 })
      .mockResolvedValueOnce({ payments: [] })

    render(<AdminPaymentsTab />)

    await waitFor(() => expect(apiFetch).toHaveBeenCalledWith('/api/admin/payments'))
    await waitFor(() => expect(apiFetch).toHaveBeenCalledWith('/api/admin/payments/yookassa/config'))
    expect(screen.getByText(/Профи · payer@test.local/)).toBeTruthy()
    expect(screen.getByText(/pending · autorenew requested · renewal #2 · enabled · bank_card\/active/)).toBeTruthy()
    expect(screen.getByText(/Активен: боевой магазин · реквизиты есть/)).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Запустить автопродления' }))

    await waitFor(() => expect(apiFetch).toHaveBeenCalledWith('/api/admin/payments/yookassa/run-due-renewals', { method: 'POST' }))
  })

  it('saves YooKassa test mode without reading raw secret back', async () => {
    apiFetch
      .mockResolvedValueOnce({ payments: [] })
      .mockResolvedValueOnce({
        testModeEnabled: false,
        activeMode: 'live',
        activeConfigured: true,
        liveConfigured: true,
        testConfigured: false,
      })
      .mockResolvedValueOnce({
        testModeEnabled: true,
        activeMode: 'test',
        activeConfigured: true,
        liveConfigured: true,
        testConfigured: true,
        testShopId: 'test-shop-1',
        testSecretKeyMasked: '****7890',
      })

    render(<AdminPaymentsTab />)

    await waitFor(() => expect(screen.getByText(/Активен: боевой магазин/)).toBeTruthy())
    fireEvent.click(screen.getByLabelText('Использовать тестовый магазин для новых платежей'))
    fireEvent.change(screen.getByLabelText('Test shopId'), { target: { value: 'test-shop-1' } })
    fireEvent.change(screen.getByLabelText('Test secret key'), { target: { value: 'secret-7890' } })
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить режим' }))

    await waitFor(() => {
      const call = apiFetch.mock.calls.find((c) => c[0] === '/api/admin/payments/yookassa/config' && c[1]?.method === 'POST')
      expect(call).toBeTruthy()
      expect(JSON.parse(String(call?.[1]?.body))).toEqual({
        testModeEnabled: true,
        testShopId: 'test-shop-1',
        clearTestSecret: false,
        testSecretKey: 'secret-7890',
      })
    })
    expect(screen.queryByDisplayValue('secret-7890')).toBeNull()
  })

  it('can run safe test auto charge and revoke subscription access from lifecycle row', async () => {
    const row = {
      id: 2,
      userId: 8,
      subscriptionId: 12,
      email: 'paid@test.local',
      tariffName: 'Команда',
      paymentId: 'yk-row',
      status: 'paid',
      amount: '300.00',
      currency: 'RUB',
      isRecurring: true,
      autoRenewRequested: true,
      renewalAttempt: 0,
      createdAt: '2026-06-05T12:00:00Z',
      autoRenewEnabled: true,
      subscriptionStatus: 'active',
      paymentMethodType: 'bank_card',
      paymentMethodStatus: 'active',
    }
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    apiFetch
      .mockResolvedValueOnce({ payments: [row] })
      .mockResolvedValueOnce({
        testModeEnabled: true,
        activeMode: 'test',
        activeConfigured: true,
        liveConfigured: true,
        testConfigured: true,
      })
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ payments: [row] })
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ payments: [] })

    render(<AdminPaymentsTab />)

    await waitFor(() => expect(screen.getByText(/Команда · paid@test.local/)).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Тест списания' }))

    await waitFor(() => {
      const call = apiFetch.mock.calls.find((c) => c[0] === '/api/admin/payments/yookassa/test-charge')
      expect(call).toBeTruthy()
      expect(JSON.parse(String(call?.[1]?.body))).toEqual({
        userId: 8,
        subscriptionId: 12,
        amountRub: 100,
        description: 'Тестовое автосписание Mindstrata',
      })
    })

    fireEvent.click(screen.getByRole('button', { name: 'Снять доступ' }))
    await waitFor(() => {
      const call = apiFetch.mock.calls.find((c) => c[0] === '/api/admin/payments/subscriptions/revoke')
      expect(call).toBeTruthy()
      expect(JSON.parse(String(call?.[1]?.body))).toEqual({
        userId: 8,
        subscriptionId: 12,
        reason: 'admin_revoked',
      })
    })
    expect(confirmSpy).toHaveBeenCalled()
    confirmSpy.mockRestore()
  })

  it('can find unused paid access and transfer it to a recovered account', async () => {
    apiFetch
      .mockResolvedValueOnce({ payments: [] })
      .mockResolvedValueOnce({
        testModeEnabled: false,
        activeMode: 'live',
        activeConfigured: true,
        liveConfigured: true,
        testConfigured: false,
      })
      .mockResolvedValueOnce({
        items: [
          {
            invoiceId: 55,
            userId: 90,
            email: 'guest@test.local',
            tariffName: 'Спикер',
            paymentId: 'yk-lost-cookie',
            amount: '100.00',
            currency: 'RUB',
            status: 'paid',
            subscriptionMonths: 1,
            accessCount: 1,
            usageCount: 0,
            transferableWithoutRisk: true,
          },
        ],
      })
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({ payments: [] })

    render(<AdminPaymentsTab />)

    fireEvent.change(await screen.findByPlaceholderText('paymentId, email, телефон, userId'), {
      target: { value: 'yk-lost-cookie' },
    })
    fireEvent.change(screen.getByPlaceholderText('куда передать: userId или email'), {
      target: { value: 'client@test.local' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Найти' }))

    await waitFor(() => expect(screen.getByText(/#55 · Спикер · guest@test.local/)).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Передать доступ' }))

    await waitFor(() => {
      const call = apiFetch.mock.calls.find((c) => c[0] === '/api/admin/payments/access-recovery' && c[1]?.method === 'POST')
      expect(call).toBeTruthy()
      expect(JSON.parse(String(call?.[1]?.body))).toEqual({
        invoiceId: 55,
        targetUserId: 0,
        targetEmail: 'client@test.local',
        reason: 'lost_cookie_recovery',
      })
    })
  })
})
