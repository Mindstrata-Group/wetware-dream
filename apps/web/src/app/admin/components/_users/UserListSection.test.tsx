import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'

import { AdminPageContext } from '../AdminPageContext'
import { UserListSection } from './UserListSection'

function makeContext(overrides: Record<string, unknown> = {}) {
  return {
    users: [],
    userDetail: null,
    grantUserDetail: null,
    userQ: '',
    setUserQ: vi.fn(),
    userMeta: { offset: 0, limit: 50, total: 0 },
    selectedUser: null,
    setSelectedUser: vi.fn(),
    created: { email: '', password: '', role: 'tester', status: 'active' },
    setCreated: vi.fn(),
    modes: [],
    statusHelp: vi.fn(() => ''),
    roleDescriptions: {},
    roles: ['tester', 'admin'],
    statuses: ['active', 'blocked'],
    pageSizes: [10, 50, 100],
    passwordScore: vi.fn(() => ({ label: 'ok', color: 'green' })),
    copy: vi.fn(),
    formatDate: vi.fn(() => '—'),
    statusLabel: vi.fn((value: string) => value),
    createUser: vi.fn(),
    patchUser: vi.fn(),
    openUserDetail: vi.fn(),
    closeUserDetail: vi.fn(),
    deleteUserAccessMode: vi.fn(),
    loadUsers: vi.fn(),
    userPage: 1,
    totalPages: 1,
    openUser: vi.fn(),
    setTab: vi.fn(),
    openMode: vi.fn(),
    ...overrides,
  }
}

function renderUserList(ctx = makeContext()) {
  return render(
    <AdminPageContext.Provider value={ctx}>
      <UserListSection />
    </AdminPageContext.Provider>,
  )
}

describe('UserListSection regressions', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('regression_loadUsers_callback_2026_06_01: click «Найти» вызывает loadUsers', () => {
    const ctx = makeContext()

    renderUserList(ctx)
    fireEvent.click(screen.getByText('Найти'))

    expect(ctx.loadUsers).toHaveBeenCalledWith(0)
  })

  it('regression_loadUsers_callback_2026_06_01: Enter в input вызывает loadUsers', () => {
    const ctx = makeContext()

    renderUserList(ctx)
    fireEvent.keyDown(screen.getByPlaceholderText(/email, username или id/i), {
      key: 'Enter',
      code: 'Enter',
    })

    expect(ctx.loadUsers).toHaveBeenCalledWith(0)
  })

  it('regression_loadUsers_callback_2026_06_01: change limit вызывает loadUsers(0, newLimit)', () => {
    const ctx = makeContext()

    renderUserList(ctx)
    fireEvent.change(screen.getByDisplayValue('50'), { target: { value: '100' } })

    expect(ctx.loadUsers).toHaveBeenCalledWith(0, 100)
  })

  it('patches user role and status from the list controls', () => {
    const user = {
      id: 42,
      email: 'person@example.com',
      role: 'tester',
      status: 'active',
      telegramUsername: '',
      messageCount: 7,
      lastLoginAt: '2026-06-05T08:00:00Z',
    }
    const ctx = makeContext({ users: [user] })

    renderUserList(ctx)
    fireEvent.change(screen.getAllByDisplayValue('tester')[0], { target: { value: 'admin' } })
    fireEvent.change(screen.getAllByDisplayValue('active')[0], { target: { value: 'blocked' } })

    expect(ctx.patchUser).toHaveBeenCalledWith(user, { role: 'admin' })
    expect(ctx.patchUser).toHaveBeenCalledWith(user, { status: 'blocked' })
  })

  it('loads the previous and next user pages from the current offset', () => {
    const ctx = makeContext({
      userMeta: { offset: 50, limit: 50, total: 125 },
      userPage: 2,
      totalPages: 3,
    })

    renderUserList(ctx)
    fireEvent.click(screen.getByRole('button', { name: /назад/i }))
    fireEvent.click(screen.getByRole('button', { name: /вперёд/i }))

    expect(ctx.loadUsers).toHaveBeenCalledWith(0)
    expect(ctx.loadUsers).toHaveBeenCalledWith(100)
  })
})
