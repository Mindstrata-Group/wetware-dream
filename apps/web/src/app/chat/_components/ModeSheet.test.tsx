import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ModeSheet } from './ModeSheet'

const modes = [
  { id: 1, name: 'Системный анализ', welcomeMessage: 'Начнём с задачи', quota: { remaining: 3, limit: 5, used: 2 } },
  { id: 2, name: 'Переговоры', welcomeMessage: 'Разберём разговор', quota: { remaining: 1, limit: 5, used: 4 } },
]

describe('ModeSheet', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows auto mode switching copy and keeps selection controls reachable', () => {
    const onToggle = vi.fn()
    const onSelectAll = vi.fn()
    const onClear = vi.fn()

    render(
      <ModeSheet
        modes={modes as any}
        activeIds={[1]}
        onToggle={onToggle}
        onSelectAll={onSelectAll}
        onClear={onClear}
        onClose={vi.fn()}
        isMobile={false}
      />,
    )

    expect(screen.getByRole('heading', { name: 'Настроить режимы' })).toBeTruthy()
    expect(screen.getByText(/Стратум сам переключится по задаче/i)).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'Выберите режимы' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Использовать все' }))
    fireEvent.click(screen.getByRole('button', { name: 'Очистить выбор' }))
    fireEvent.click(screen.getByRole('button', { name: /Переговоры/ }))

    expect(onSelectAll).toHaveBeenCalledTimes(1)
    expect(onClear).toHaveBeenCalledTimes(1)
    expect(onToggle).toHaveBeenCalledWith(2)
  })
})
