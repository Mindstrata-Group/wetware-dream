import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'

vi.mock('next/dynamic', () => ({
  default: (_loader: unknown, options?: { loading?: () => ReactNode }) => {
    const DynamicMock = () => <>{options?.loading ? options.loading() : <div />}</>
    return DynamicMock
  },
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

vi.mock('@/components/BrandLogo', () => ({
  BrandLogo: () => <a href="/" aria-label="СТРАТУМ Mindstrata">logo</a>,
}))

import { AdminPageShell } from './AdminPageShell'

function makeController(overrides: Record<string, unknown> = {}) {
  return {
    adminPageContext: {} as any,
    headerMobile: true,
    visibleTabs: [
      { id: 'stats', label: 'Статистика' },
      { id: 'users', label: 'Пользователи' },
      { id: 'promocodes', label: 'Промокоды' },
      { id: 'modes', label: 'Режимы' },
    ],
    tab: 'stats',
    selectTab: vi.fn(),
    error: '',
    errorDebug: null,
    notice: '',
    logout: vi.fn(),
    ...overrides,
  } as any
}

describe('AdminPageShell mobile navigation', () => {
  it('does not render removed mobile bottom quick navigation', () => {
    const { container } = render(<AdminPageShell controller={makeController()} />)

    expect(container.querySelector('.ms-mobile-bottom-nav')).toBeNull()
  })

  it('keeps the top mobile strip navigation available instead of bottom quick nav', () => {
    render(<AdminPageShell controller={makeController()} />)

    const strip = screen.getByRole('navigation', { name: /Разделы/i })
    expect(strip).toBeTruthy()
    expect(within(strip).getByRole('button', { name: /Стат/i })).toBeTruthy()
    expect(within(strip).getByRole('button', { name: /Польз/i })).toBeTruthy()
  })
})
