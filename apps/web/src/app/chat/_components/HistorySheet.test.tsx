import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { HistorySheet } from './HistorySheet'

const mockHistoryItem = {
  dialogId: 101,
  title: 'Тестовый диалог',
  modeName: 'Системный анализ',
  updatedAt: '2026-09-30T10:00:00Z',
  pinned: false,
  snippet: 'Последнее сообщение',
}

describe('HistorySheet accessibility', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders icon-only buttons with accessible aria-label and aria-expanded attributes', () => {
    const onRestore = vi.fn()
    const onDelete = vi.fn()
    const onTogglePin = vi.fn()
    const onRename = vi.fn()
    const onClear = vi.fn()
    const onClose = vi.fn()
    const setSearch = vi.fn()

    render(
      <HistorySheet
        items={[mockHistoryItem]}
        currentDialogId={101}
        onRestore={onRestore}
        onDelete={onDelete}
        onTogglePin={onTogglePin}
        onRename={onRename}
        onClear={onClear}
        onClose={onClose}
        search=""
        setSearch={setSearch}
      />,
    )

    const menuButton = screen.getByRole('button', { name: 'Меню' })
    expect(menuButton).toBeTruthy()
    expect(menuButton.getAttribute('aria-expanded')).toBe('false')

    fireEvent.click(menuButton)
    expect(menuButton.getAttribute('aria-expanded')).toBe('true')

    const closeButton = screen.getByRole('button', { name: 'Закрыть' })
    expect(closeButton).toBeTruthy()

    const deleteButton = screen.getByRole('button', { name: 'Удалить' })
    expect(deleteButton).toBeTruthy()

    fireEvent.click(deleteButton)
    expect(onDelete).toHaveBeenCalledWith(101)
  })
})
