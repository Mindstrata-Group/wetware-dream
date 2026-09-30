import { describe, it, expect, vi, afterEach } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'

import { SYSTEM_ACTIONS, SYSTEM_LINKS } from '@/lib/opsLinks'
import { AdminPageContext } from './AdminPageContext'
import { AdminSystemTab } from './AdminSystemTab'

function jsonResponse(body: Record<string, unknown>, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response
}

// On mount the tab loads /api/admin/analytics-settings; fetch is always mocked.
function renderSystemTab(
  overrides: Record<string, unknown> = {},
  fetchMock = vi.fn(async () =>
    jsonResponse({ ok: true, metrikaCounterId: '', metrikaParams: '' }),
  ),
) {
  vi.stubGlobal('fetch', fetchMock)

  const ctx = {
    status: { counts: { users: 12, activeUsers: 7, messages: 3456 } },
    loadSystem: vi.fn(),
    ...overrides,
  } as any

  const view = render(
    <AdminPageContext.Provider value={ctx}>
      <AdminSystemTab />
    </AdminPageContext.Provider>,
  )

  return { ...view, ctx, fetchMock }
}

describe('AdminSystemTab ops links', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders only the approved service panel links and no Ollama/services table', async () => {
    const { container } = renderSystemTab()
    // wait for the Metrica form to load so the state update does not fail outside act()
    await screen.findByPlaceholderText(/например 98765432/)

    // Dashboard addresses are installation settings (lib/opsLinks); the test
    // checks rendering against them, not against particular subdomains.
    const panels = SYSTEM_LINKS.length > 0 ? screen.getByLabelText('Служебные панели') : null
    if (!panels) expect(screen.queryByLabelText('Служебные панели')).toBeNull()
    else expect(within(panels).getAllByRole('link')).toHaveLength(SYSTEM_LINKS.length)
    for (const { label, href } of SYSTEM_LINKS) {
      const link = within(panels!).getByRole('link', { name: label })
      expect(link.getAttribute('href')).toBe(href)
      expect(link.getAttribute('target')).toBe('_blank')
      expect(link.getAttribute('rel')).toBe('noreferrer')
    }

    expect(screen.queryByText(/ollama/i)).toBeNull()
    expect(screen.queryByRole('link', { name: 'Dozzle' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Metrics' })).toBeNull()
    expect(container.querySelector('table')).toBeNull()
  })

  it('exposes the Main to Stage recovery workflow and refresh action', async () => {
    const { ctx } = renderSystemTab()
    await screen.findByPlaceholderText(/например 98765432/)

    if (SYSTEM_ACTIONS.length === 0) {
      expect(screen.queryByLabelText('Операции окружения')).toBeNull()
    } else {
      const actions = screen.getByLabelText('Операции окружения')
      for (const { label, href, title } of SYSTEM_ACTIONS) {
        const link = within(actions).getByRole('link', { name: label })
        expect(link.getAttribute('href')).toBe(href)
        expect(link.getAttribute('title')).toBe(title ?? null)
      }
    }

    fireEvent.click(screen.getByRole('button', { name: 'Обновить' }))
    expect(ctx.loadSystem).toHaveBeenCalledTimes(1)
  })
})

describe('AdminSystemTab Яндекс.Метрика', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('loads saved counter id and params into the form', async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({ ok: true, metrikaCounterId: '98765432', metrikaParams: '{"webvisor":true}' }),
    )
    renderSystemTab({}, fetchMock)

    await waitFor(() => {
      expect(screen.getByDisplayValue('98765432')).toBeTruthy()
    })
    expect(screen.getByDisplayValue('{"webvisor":true}')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/admin/analytics-settings'),
      expect.objectContaining({ credentials: 'include' }),
    )
  })

  it('saves edited counter id via POST', async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') return jsonResponse({ ok: true })
      return jsonResponse({ ok: true, metrikaCounterId: '', metrikaParams: '' })
    })
    renderSystemTab({}, fetchMock)

    const input = await screen.findByPlaceholderText(/например 98765432/)
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false))
    fireEvent.change(input, { target: { value: '11112222' } })
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))

    await waitFor(() => {
      expect(screen.getByText(/Сохранено/)).toBeTruthy()
    })
    const postCall = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'POST')
    expect(postCall).toBeTruthy()
    expect(JSON.parse((postCall![1] as RequestInit).body as string)).toEqual({
      metrikaCounterId: '11112222',
      metrikaParams: '',
    })
  })

  it('shows backend validation error on save failure', async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return jsonResponse({ ok: false, error: 'номер счётчика — только цифры (или пусто, чтобы выключить)' }, 400)
      }
      return jsonResponse({ ok: true, metrikaCounterId: '', metrikaParams: '' })
    })
    renderSystemTab({}, fetchMock)

    const input = await screen.findByPlaceholderText(/например 98765432/)
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false))
    fireEvent.change(input, { target: { value: 'abc' } })
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))

    await waitFor(() => {
      expect(screen.getByText(/только цифры/)).toBeTruthy()
    })
  })
})
