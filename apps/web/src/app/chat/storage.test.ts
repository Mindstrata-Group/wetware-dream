import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mode } from './types'
import {
  HISTORY_KEY,
  LAST_SELECTED_MODE_IDS_KEY,
  SELECTED_MODE_IDS_KEY,
  buildHistorySearchText,
  buildHistorySnippet,
  buildHistoryTitle,
  readHistory,
  readPendingSelectedModeIds,
  readStoredSelectedModeIds,
  uniqueAvailableModeIds,
  writeHistory,
  writeStoredSelectedModeIds,
} from './storage'

const modes: Mode[] = [
  { id: 10, name: 'Mode 10' },
  { id: 20, name: 'Mode 20' },
]

describe('chat storage P0', () => {
  beforeEach(() => {
    localStorage.clear()
    window.history.pushState(null, '', '/')
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('deduplicates mode ids and drops ids unavailable to the current user', () => {
    expect(uniqueAvailableModeIds([10, 10, 999, Number.NaN, 20], modes)).toEqual([10, 20])
  })

  it('reads one-shot selected modes from query and clears the URL marker/storage', () => {
    localStorage.setItem(SELECTED_MODE_IDS_KEY, JSON.stringify([20]))
    window.history.pushState(null, '', '/chat?modes=10,999,20')

    expect(readPendingSelectedModeIds(modes)).toEqual([10, 20])
    expect(window.location.search).toBe('')
    expect(localStorage.getItem(SELECTED_MODE_IDS_KEY)).toBeNull()
  })

  it('reads one-shot selected modes from localStorage and removes malformed/stale values safely', () => {
    localStorage.setItem(SELECTED_MODE_IDS_KEY, JSON.stringify([20, 'bad', 999]))

    expect(readPendingSelectedModeIds(modes)).toEqual([20])
    expect(localStorage.getItem(SELECTED_MODE_IDS_KEY)).toBeNull()
  })

  it('distinguishes deliberately empty stored selection from missing storage', () => {
    expect(readStoredSelectedModeIds(modes)).toEqual({ ids: [], exists: false })
    localStorage.setItem(LAST_SELECTED_MODE_IDS_KEY, JSON.stringify([]))
    expect(readStoredSelectedModeIds(modes)).toEqual({ ids: [], exists: true })
  })

  it('writes and reads stable selected modes', () => {
    writeStoredSelectedModeIds([10, 20])
    expect(readStoredSelectedModeIds(modes)).toEqual({ ids: [10, 20], exists: true })
  })

  it('round-trips history and ignores corrupted cache', () => {
    const items = [{ dialogId: 1, modeId: 10, modeName: 'Mode 10', title: 'T', snippet: 'S', searchText: 't s', pinned: false, updatedAt: 'now' }]
    writeHistory(items)
    expect(readHistory()).toEqual(items)
    localStorage.setItem(HISTORY_KEY, '{bad')
    expect(readHistory()).toEqual([])
  })

  it('buildHistorySearchText lowercases all content (убивает toLowerCase мутацию)', () => {
    expect(buildHistorySearchText([
      { id: 1, role: 'user' as const, content: 'ЗАГЛАВНЫЕ', createdAt: 'now' },
    ])).toBe('user заглавные')
  })

  it('builds resilient history title/snippet/search text', () => {
    const messages = [
      { id: 1, role: 'assistant' as const, content: 'Здравствуйте', createdAt: 'now' },
      { id: 2, role: 'user' as const, content: '  Очень длинный пользовательский вопрос, который должен обрезаться после лимита символов  ', createdAt: 'now' },
      { id: 3, role: 'assistant' as const, content: 'Ответ\nс пробелами', createdAt: 'now' },
    ]

    expect(buildHistoryTitle('Mode Name', messages, 99)).toHaveLength(56)
    expect(buildHistorySnippet(messages)).toBe('Ответ с пробелами')
    expect(buildHistorySearchText(messages)).toContain('user очень длинный пользовательский вопрос')
    expect(buildHistoryTitle('', [], 99)).toBe('Диалог 99')
  })

  it('falls back to modeName when messages has no user message', () => {
    // kills: if (modeName.trim()) -> if (false)
    const assistantOnly = [
      { id: 1, role: 'assistant' as const, content: 'Приветствую', createdAt: 'now' },
    ]
    expect(buildHistoryTitle('Мой режим', assistantOnly, 7)).toBe('Мой режим')
    // empty modeName -> dialog id
    expect(buildHistoryTitle('   ', assistantOnly, 7)).toBe('Диалог 7')
  })

  it('finds last message from any role in snippet, including summary', () => {
    // kills: m.role === "summary" -> false
    const withSummary = [
      { id: 1, role: 'user' as const, content: 'Вопрос', createdAt: 'now' },
      { id: 2, role: 'summary' as const, content: 'Краткое содержание диалога', createdAt: 'now' },
    ]
    expect(buildHistorySnippet(withSummary)).toBe('Краткое содержание диалога')
    // empty messages -> empty string (kills the || "" mutation)
    expect(buildHistorySnippet([])).toBe('')
  })

  it('finds user as last message in snippet', () => {
    // kills: m.role === "user" -> false (the last message from the user is skipped)
    expect(buildHistorySnippet([
      { id: 1, role: 'assistant' as const, content: 'Ответ', createdAt: 'now' },
      { id: 2, role: 'user' as const, content: 'Последний вопрос', createdAt: 'now' },
    ])).toBe('Последний вопрос')
  })

  it('trims snippet to 150 chars', () => {
    // kills: slice(0, 150) -> slice(0, 0) or slice(0, 151)
    const longContent = 'а'.repeat(200)
    const result = buildHistorySnippet([{ id: 1, role: 'assistant' as const, content: longContent, createdAt: 'now' }])
    expect(result.length).toBe(150)
  })

  it('trims whitespace from modeName when returning it as title', () => {
    // kills: return modeName -> return modeName.trim() (without trim it returns the name with its surrounding spaces)
    expect(buildHistoryTitle('  Режим  ', [], 5)).toBe('Режим')
  })

  it('filters NaN from query string mode ids', () => {
    // kills: Number.isFinite(v) -> true (NaN from 'abc' would end up in the result)
    window.history.pushState(null, '', '/chat?modes=10,abc,20')
    expect(readPendingSelectedModeIds(modes)).toEqual([10, 20])
  })

  it('marks stored ids as not-exists when all stored ids are unavailable', () => {
    // kills: ids.length === 0 || validIds.length > 0 -> true
    localStorage.setItem(LAST_SELECTED_MODE_IDS_KEY, JSON.stringify([999, 888]))
    expect(readStoredSelectedModeIds(modes)).toEqual({ ids: [], exists: false })
  })
})
