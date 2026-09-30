import { describe, it, expect, vi, afterEach } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { AdminPageContext } from '../AdminPageContext'
import { FilterColumns } from './FilterColumns'

function users(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    id: index + 1,
    email: `user${index + 1}@test.local`,
    telegramUsername: '',
    createdAt: '2026-06-02T00:00:00Z',
    messageCount: index,
  }))
}

function makeContext(overrides: Record<string, unknown> = {}) {
  return {
    exportSort: {},
    exportFilters: { modeIds: [], userIds: [], userSearch: '', modeSearch: '', promocodeIds: [], promocodeSearch: '', promptId: '', customPrompt: '', withSummary: false, roleFilter: 'all', limit: '', dateFrom: '', dateTo: '' },
    setExportFilters: vi.fn(),
    filteredUsers: users(0),
    filteredPromos: [],
    filteredExportModes: [],
    promoModeSet: null,
    sortButton: (label: string) => <button type="button">{label}</button>,
    toggleExportSort: vi.fn(),
    formatDate: () => '02.06.2026',
    promoStateLabel: () => 'активен',
    toggleNumberSelection: (ids: number[], id: number) => ids.includes(id) ? ids.filter((value) => value !== id) : [...ids, id],
    ...overrides,
  } as any
}

function renderFilterColumns(ctx = makeContext()) {
  return render(
    <AdminPageContext.Provider value={ctx}>
      <FilterColumns />
    </AdminPageContext.Provider>,
  )
}

describe('FilterColumns pagination', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('clamps opened table page when filtered rows shrink', async () => {
    const { rerender } = renderFilterColumns(makeContext({ filteredUsers: users(15) }))
    fireEvent.click(screen.getAllByRole('button', { name: /Открыть последние/i })[1])
    fireEvent.click(screen.getByRole('button', { name: /Вперёд/i }))
    expect(screen.getByText('11–15 из 15')).toBeTruthy()

    rerender(
      <AdminPageContext.Provider value={makeContext({ filteredUsers: users(5) })}>
        <FilterColumns />
      </AdminPageContext.Provider>,
    )

    expect(await screen.findByText('1–5 из 5')).toBeTruthy()
    expect((screen.getByRole('button', { name: /Вперёд/i }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('shows empty state without pagination after opening an empty table', () => {
    renderFilterColumns(makeContext({ filteredUsers: [] }))
    fireEvent.click(screen.getAllByRole('button', { name: /Открыть последние/i })[1])

    expect(screen.getByText('Пользователей не найдено')).toBeTruthy()
    expect(screen.queryByText(/из 0/)).toBeNull()
    expect(screen.queryByRole('button', { name: /Вперёд/i })).toBeNull()
  })

  it('does not render the whole users table before opening or beyond one page', () => {
    renderFilterColumns(makeContext({ filteredUsers: users(25) }))

    expect(screen.queryByText('user1@test.local')).toBeNull()
    expect(screen.queryByText('user25@test.local')).toBeNull()
    expect(screen.getByText(/Найдено: 25/)).toBeTruthy()

    fireEvent.click(screen.getAllByRole('button', { name: /Открыть последние/i })[1])

    expect(screen.getByText('user1@test.local')).toBeTruthy()
    expect(screen.getByText('user10@test.local')).toBeTruthy()
    expect(screen.queryByText('user11@test.local')).toBeNull()
    expect(screen.queryByText('user25@test.local')).toBeNull()
    expect(screen.getByText('1–10 из 25')).toBeTruthy()
  })
})
