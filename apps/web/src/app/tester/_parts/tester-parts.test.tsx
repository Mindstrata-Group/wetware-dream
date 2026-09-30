import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildChecksFromStatus } from './buildChecksFromStatus'
import { CheckRow } from './CheckRow'
import { computeStats } from './computeStats'
import { HScrollTabs } from './HScrollTabs'
import { StatusIcon } from './StatusIcon'

describe('tester status helpers', () => {
  it('builds no checks while status is missing', () => {
    expect(buildChecksFromStatus(null)).toEqual([])
  })

  it('builds observable checks with request/response debug JSON from system counts', () => {
    expect(buildChecksFromStatus({ counts: { users: 12, modes: 3 } })).toEqual([
      {
        id: 'system',
        label: 'Система',
        checks: [
          {
            id: 'system-users',
            name: 'users',
            status: 'ok',
            detail: '12',
            json: { request: { method: 'GET', endpoint: '/api/admin/status' }, response: { users: 12 } },
          },
          {
            id: 'system-modes',
            name: 'modes',
            status: 'ok',
            detail: '3',
            json: { request: { method: 'GET', endpoint: '/api/admin/status' }, response: { modes: 3 } },
          },
        ],
      },
    ])
  })

  it('adds a fallback service check when counts are empty', () => {
    const groups = buildChecksFromStatus({ counts: {} })
    expect(groups[0]?.checks).toEqual([
      { id: 'system-ok', name: 'Сервис работает', status: 'ok', detail: 'API доступен' },
    ])
  })

  it('computes pass/warn/fail totals across groups', () => {
    expect(computeStats([
      { id: 'a', label: 'A', checks: [{ id: '1', name: 'ok', status: 'ok' }, { id: '2', name: 'warn', status: 'warn' }] },
      { id: 'b', label: 'B', checks: [{ id: '3', name: 'fail', status: 'fail' }] },
    ])).toEqual({ totalChecks: 3, passed: 1, warned: 1, failed: 1 })
  })
})

describe('tester status presentation', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it.each([
    ['ok', '✓'],
    ['warn', '!'],
    ['fail', '✕'],
  ] as const)('renders %s status icon', (status, glyph) => {
    render(<StatusIcon status={status} />)
    expect(screen.getByText(glyph)).toBeTruthy()
  })

  it('renders check details and debug JSON when present', () => {
    render(<CheckRow check={{ id: 'c1', name: 'API health', status: 'ok', detail: '200 OK', json: { ok: true } }} />)

    expect(screen.getByText('API health')).toBeTruthy()
    expect(screen.getByText('200 OK')).toBeTruthy()
    fireEvent.click(screen.getByText('API health'))
    expect(screen.getByText(/"ok": true/)).toBeTruthy()
  })

  it('renders horizontal tabs and marks the selected tab', () => {
    render(<HScrollTabs groups={[{ id: 'system', label: 'Система', checks: [] }, { id: 'auth', label: 'Auth', checks: [{ id: 'w', name: 'warn', status: 'warn' }] }]} active="auth" onSelect={() => {}} />)

    expect(screen.getByRole('button', { name: 'Система' }).style.border).toContain('var(--line)')
    expect(screen.getByRole('button', { name: /Auth/ }).style.border).toContain('rgb(29, 158, 117)')
    expect(screen.getByText('1')).toBeTruthy()
  })
})
