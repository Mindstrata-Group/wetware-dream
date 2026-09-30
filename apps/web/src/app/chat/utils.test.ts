import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Mode } from './types'
import {
  buildQuotaWarning,
  cleanOutgoingMessage,
  collapseAdjacentModeSwitchTraces,
  dailyUnlockLabel,
  displaySystemMessage,
  formatDate,
  formatQuota,
  modeSwitchTraceContent,
  modeHasMessages,
  prependUniqueMessages,
  quotaExhausted,
  quotaNearLimit,
  quotaRemaining,
  replaceTrailingModeSwitchTrace,
  selectableModeIds,
  syncModesWithGlobalUsage,
} from './utils'

const modes: Mode[] = [
  { id: 1, name: 'A', quota: { used: 2, limit: 5, remaining: 3 } },
  { id: 2, name: 'B', quota: { used: 5, limit: 5, remaining: 0 } },
  { id: 3, name: 'C' },
]

describe('chat utils P0', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('formats quota variants without throwing on partial backend payloads', () => {
    expect(formatQuota(null)).toBe('Лимит: —')
    // kills: typeof remaining === "number" -> false (remaining=4 != limit-used=7)
    expect(formatQuota({ used: 3, limit: 10, remaining: 4 })).toBe('Осталось 4 из 10')
    expect(formatQuota({ used: 3, limit: 10, remaining: 7 })).toBe('Осталось 7 из 10')
    expect(formatQuota({ used: 3, limit: 10 })).toBe('Осталось 7 из 10')
    expect(formatQuota({ used: 3 })).toBe('Использовано 3')
  })

  it('blocks only modes with explicit exhausted remaining counter', () => {
    expect(quotaExhausted({ used: 5, limit: 5, remaining: 0 })).toBe(true)
    expect(quotaExhausted({ used: 5, limit: 5 })).toBe(false)
    expect(modeHasMessages(modes[0])).toBe(true)
    expect(modeHasMessages(modes[1])).toBe(false)
    expect(selectableModeIds(modes)).toEqual([1, 3])
  })

  it('formats mode-switch traces for subtle chat diagnostics', () => {
    const content = modeSwitchTraceContent('Уточнить фразу', 'Переговоры')
    expect(content).toBe('[mode-switch] Стратум переключил режим: «Уточнить фразу» → «Переговоры».')
    expect(displaySystemMessage(content)).toBe('Стратум переключил режим: «Уточнить фразу» → «Переговоры».')
    expect(displaySystemMessage('Обычное системное событие')).toBe('Обычное системное событие')
  })

  it('keeps only the latest adjacent mode-switch trace in the chat feed', () => {
    const first = { id: 1, role: 'system', content: '[mode-switch] Режим переключён вручную: «A» → «B».', createdAt: '2026-06-01T10:00:00Z' }
    const second = { id: 2, role: 'system', content: '[mode-switch] Режим переключён вручную: «B» → «C».', createdAt: '2026-06-01T10:00:01Z' }
    const user = { id: 3, role: 'user', content: 'после переключения', createdAt: '2026-06-01T10:00:02Z' }

    expect(replaceTrailingModeSwitchTrace([first], second)).toEqual([second])
    expect(replaceTrailingModeSwitchTrace([first, user], second)).toEqual([first, user, second])
    expect(collapseAdjacentModeSwitchTraces([first, second, user])).toEqual([second, user])
  })

  it('does not duplicate messages when a paginated history page overlaps', () => {
    const older = { id: 1, role: 'user', content: 'раннее сообщение', createdAt: '2026-06-01T10:00:00Z' }
    const current = { id: 2, role: 'assistant', content: 'текущий ответ', createdAt: '2026-06-01T10:01:00Z' }
    const duplicate = { ...older, content: 'повтор из beforeId-страницы' }

    expect(prependUniqueMessages([older, duplicate], [older, current])).toEqual([older, current])
    expect(prependUniqueMessages([older], [current])).toEqual([older, current])
  })

  it('syncs mode quotas from backend global usage without exhausting larger limits', () => {
    expect(syncModesWithGlobalUsage(modes, { used: 4, limit: 10, remaining: 6 })).toEqual([
      { id: 1, name: 'A', quota: { used: 4, limit: 5, remaining: 1 } },
      { id: 2, name: 'B', quota: { used: 4, limit: 5, remaining: 1 } },
      { id: 3, name: 'C', quota: { used: 4, limit: 10, remaining: 6 } },
    ])
  })

  it('uses each mode limit so fallback modes can stay available after one mode is exhausted', () => {
    expect(syncModesWithGlobalUsage([
      { id: 1, name: 'Short', quota: { used: 50, limit: 50, remaining: 0 } },
      { id: 2, name: 'Long', quota: { used: 50, limit: 100, remaining: 50 } },
    ], { used: 50, limit: 50, remaining: 0 })).toEqual([
      { id: 1, name: 'Short', quota: { used: 50, limit: 50, remaining: 0 } },
      { id: 2, name: 'Long', quota: { used: 50, limit: 100, remaining: 50 } },
    ])
  })

  it('trusts backend global usage over stale per-mode cache', () => {
    expect(syncModesWithGlobalUsage(modes, { used: 1, limit: 10, remaining: 9 })[0].quota).toEqual({
      used: 1,
      limit: 5,
      remaining: 4,
    })
  })

  it('detects near-limit quota without treating exhausted quota as a warning', () => {
    // kills: typeof remaining === "number" -> false (remaining field vs computed)
    expect(quotaRemaining({ used: 3, limit: 10, remaining: 4 })).toBe(4)
    expect(quotaRemaining({ used: 8, limit: 10 })).toBe(2)
    expect(quotaRemaining(null)).toBe(null)
    expect(quotaRemaining(undefined)).toBe(null)
    expect(quotaRemaining({ used: 3 })).toBe(null)
    expect(quotaNearLimit({ used: 8, limit: 10, remaining: 2 }, { thresholdRemaining: 3 })).toBe(true)
    expect(quotaNearLimit({ used: 10, limit: 10, remaining: 0 }, { thresholdRemaining: 3 })).toBe(false)
    expect(quotaNearLimit({ used: 8, limit: 10, remaining: 2 }, { enabled: false, thresholdRemaining: 3 })).toBe(false)
    expect(quotaNearLimit(null, { thresholdRemaining: 3 })).toBe(false)
    // kills: thresholdRemaining ?? 3 -> ?? 0 (default not covered)
    // remaining=3, absolute=max(1,floor(3))=3, percent=max(1,ceil(10*20/100))=2, remaining≤3 → true
    expect(quotaNearLimit({ used: 7, limit: 10, remaining: 3 }, {})).toBe(true)
    // kills: thresholdPercent ?? 20 -> ?? 0 (default not covered)
    // remaining=3, thresholdRemaining=1, percent=max(1,ceil(20*20/100))=4, 3≤max(1,4)=4 → true
    // with ?? 0: percent=max(1,0)=1, 3>max(1,1)=1 -> false
    expect(quotaNearLimit({ used: 17, limit: 20, remaining: 3 }, { thresholdRemaining: 1 })).toBe(true)
  })

  it('builds user-facing quota warning text from CMS templates', () => {
    expect(buildQuotaWarning(
      { used: 8, limit: 10, remaining: 2 },
      {
        thresholdRemaining: 2,
        thresholdPercent: 10,
        title: 'Осталось {{remaining}}',
        body: 'Можно отправить ещё {{remaining}} из {{limit}}.',
      },
    )).toEqual({
      title: 'Осталось 2',
      body: 'Можно отправить ещё 2 из 10.',
      remaining: 2,
      limit: 10,
    })
  })

  it('uses percent-based near-limit threshold independent of absolute count', () => {
    // percent = max(1, ceil(20 * max(0,20) / 100)) = 4; absolute = 1; remaining=3 ≤ 4 → true
    // kills the mutations: typeof->false, Math.max->Math.min, *->/
    expect(quotaNearLimit({ used: 17, limit: 20, remaining: 3 }, { thresholdRemaining: 1, thresholdPercent: 20 })).toBe(true)
    expect(quotaNearLimit({ used: 15, limit: 20, remaining: 5 }, { thresholdRemaining: 1, thresholdPercent: 20 })).toBe(false)
  })

  it('substitutes template vars with spaces around the key name', () => {
    // kills the regex mutation \s* -> \S*: '{{ remaining }}' would not match \S*}}
    expect(buildQuotaWarning(
      { used: 8, limit: 10, remaining: 2 },
      { thresholdRemaining: 5, title: '{{ remaining }} left', body: '{{used}}' },
    )).toMatchObject({ title: '2 left', body: '8' })
  })

  it('leaves unknown template keys unchanged', () => {
    // kills the mutation value === undefined ? match -> false ? match (would return String(undefined))
    expect(buildQuotaWarning(
      { used: 8, limit: 10, remaining: 2 },
      { thresholdRemaining: 5, title: '{{unknown_key}}', body: 'B' },
    )?.title).toBe('{{unknown_key}}')
  })

  it('cleans outgoing chat messages from zero-width chars and excessive blanks', () => {
    expect(cleanOutgoingMessage('  Привет\u200B  \n\n\n  мир  \n  ')).toBe('Привет\n\nмир')
    // kills regex narrowing mutations: \uFEFF and \u200C are removed too
    expect(cleanOutgoingMessage('hello\uFEFFworld')).toBe('helloworld')
    expect(cleanOutgoingMessage('a\u200Cb')).toBe('ab')
  })

  it('calculates unlock label until next local midnight', () => {
    vi.useFakeTimers()
    // Use the local Date constructor: dailyUnlockLabel deliberately counts to local midnight.
    vi.setSystemTime(new Date(2026, 4, 31, 22, 30, 0))
    expect(dailyUnlockLabel()).toBe('1 ч 30 мин')
    vi.setSystemTime(new Date(2026, 4, 31, 23, 0, 0))
    expect(dailyUnlockLabel()).toBe('1 ч')
    vi.setSystemTime(new Date(2026, 4, 31, 23, 59, 30))
    expect(dailyUnlockLabel()).toBe('1 мин')
  })

  it('formats absent dates as dash for history/profile panels', () => {
    expect(formatDate(null)).toBe('—')
    expect(formatDate(undefined)).toBe('—')
    // kills the value ? ... : "—" mutation via a non-zero value
    const result = formatDate('2026-06-01T00:00:00Z')
    expect(result).not.toBe('—')
    expect(typeof result).toBe('string')
  })

  it('substitutes empty string for limit/used when quota fields are absent', () => {
    // kills the mutation quota?.limit ?? "" -> ?? 0 (limit absent -> empty string, not '0')
    expect(buildQuotaWarning(
      { used: 8, remaining: 2 },
      { thresholdRemaining: 5, title: 'Лимит: {{limit}}', body: 'Использовано {{used}}' },
    )).toMatchObject({ title: 'Лимит: ', body: 'Использовано 8' })
  })

  it('builds null warning when quota is outside threshold', () => {
    expect(buildQuotaWarning(
      { used: 3, limit: 10, remaining: 7 },
      { thresholdRemaining: 2, title: 'T', body: 'B' },
    )).toBe(null)
  })

  it('fills template vars with case-insensitive key fallback', () => {
    expect(buildQuotaWarning(
      { used: 8, limit: 10, remaining: 2 },
      {
        thresholdRemaining: 5,
        title: '{{REMAINING}} left',
        body: 'used: {{USED}}',
      },
    )).toMatchObject({
      title: '2 left',
      body: 'used: 8',
    })
  })

  it('falls back to global quota spread when mode has no per-mode limit', () => {
    // kills: typeof globalQuota.used -> true (incomingUsed does not default to 0)
    expect(syncModesWithGlobalUsage(
      [{ id: 9, name: 'X', quota: { used: 0, limit: 5, remaining: 5 } }],
      { limit: 5 },
    )[0].quota?.remaining).toBe(5)

    // kills: if (false) instead of if (typeof limit !== "number") (the branch without a numeric limit)
    expect(syncModesWithGlobalUsage(
      [{ id: 9, name: 'Limitless' }],
      { used: 3 },
    )[0].quota?.remaining).toBeUndefined()
  })

  it('clamps remaining to 0 when global used exceeds mode limit', () => {
    // kills the mutation Math.max(0, ...) -> Math.min(0, ...) in syncModesWithGlobalUsage
    expect(syncModesWithGlobalUsage(
      [{ id: 1, name: 'Small', quota: { used: 0, limit: 3 } }],
      { used: 10, limit: 15 },
    )[0].quota?.remaining).toBe(0)
  })

  it('clamps absolute threshold to minimum 1 when thresholdRemaining is 0', () => {
    // kills the mutation Math.max(1, floor) -> Math.max(0, floor) in quotaNearLimit
    // remaining=1, absolute=max(1,0)=1, percent=0 (no limit); 1 ≤ max(1,0)=1 → true
    // with the max(0) mutation: absolute=0, 1 > 0 -> false
    expect(quotaNearLimit({ used: 9, remaining: 1 }, { thresholdRemaining: 0, thresholdPercent: 0 })).toBe(true)
  })

  it('floor rounds fractional thresholdRemaining down, not up', () => {
    // kills the mutation Math.floor -> Math.ceil in quotaNearLimit
    // remaining=3, thresholdRemaining=2.7: floor=2, ceil=3
    // absolute=max(1,2)=2, percent=max(1,ceil(10*20/100))=2; max(2,2)=2; 3 > 2 → false
    // with ceil: absolute=max(1,3)=3; 3 <= 3 -> true
    expect(quotaNearLimit({ used: 7, limit: 10, remaining: 3 }, { thresholdRemaining: 2.7 })).toBe(false)
  })

  it('quotaExhausted returns false for null/undefined', () => {
    expect(quotaExhausted(null)).toBe(false)
    expect(quotaExhausted(undefined)).toBe(false)
  })
})
