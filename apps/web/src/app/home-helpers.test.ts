import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { OPERATOR, OPERATOR_TELEGRAM_URL } from '@/lib/operator'
import {
  TEST_ACCESS_MESSAGE,
  buildTelegramAppUrl,
  buildTelegramUrl,
  parseDemoChat,
  resolveDemoChat,
  shuffle,
  type DemoMode,
} from './home-helpers'

describe('home demo helpers', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('parses demo chat from JSON array and normalizes roles/content with a four-message limit', () => {
    const mode: DemoMode = {
      id: 1,
      name: 'demo',
      demoChat: JSON.stringify([
        { role: 'user', content: '  Привет  ' },
        { role: 'assistant', text: ' Ответ ' },
        { role: 'system', content: ' becomes user ' },
        { role: 'assistant', content: 'fourth' },
        { role: 'user', content: 'fifth is hidden' },
        { role: 'assistant', content: '   ' },
      ]),
    }

    expect(parseDemoChat(mode)).toEqual([
      { role: 'user', content: 'Привет' },
      { role: 'assistant', content: 'Ответ' },
      { role: 'user', content: 'becomes user' },
      { role: 'assistant', content: 'fourth' },
    ])
  })

  it('parses demo chat from JSON messages object and prefers demoChat over snake_case fallback', () => {
    const mode: DemoMode = {
      id: 2,
      name: 'demo',
      demo_chat: 'user: should not be used',
      demoChat: JSON.stringify({ messages: [{ role: 'assistant', content: 'JSON wins' }] }),
    }

    expect(resolveDemoChat(mode)).toBe(mode.demoChat)
    expect(parseDemoChat(mode)).toEqual([{ role: 'assistant', content: 'JSON wins' }])
  })

  it('falls back to plain text parsing with localized prefixes and alternating roles', () => {
    expect(parseDemoChat({
      id: 3,
      name: 'plain',
      demo_chat: 'Пользователь: запрос\nАссистент: ответ\nБез префикса\nПомощник стратума: подсказка\nлишнее',
    })).toEqual([
      { role: 'user', content: 'запрос' },
      { role: 'assistant', content: 'ответ' },
      { role: 'user', content: 'Без префикса' },
      { role: 'assistant', content: 'подсказка' },
    ])
  })

  it('returns an empty demo for missing, blank, malformed-empty payloads', () => {
    expect(parseDemoChat()).toEqual([])
    expect(parseDemoChat({ id: 4, name: 'blank', demoChat: '   ' })).toEqual([])
    expect(parseDemoChat({ id: 5, name: 'bad', demoChat: '{"messages":[]}' })).toEqual([{ role: 'user', content: '{"messages":[]}' }])
  })

  it('builds telegram web/app URLs and handles blank messages safely', () => {
    expect(buildTelegramUrl('  привет мир  ')).toBe(OPERATOR_TELEGRAM_URL + '?text=%D0%BF%D1%80%D0%B8%D0%B2%D0%B5%D1%82%20%D0%BC%D0%B8%D1%80')
    expect(buildTelegramUrl('   ')).toBe(OPERATOR_TELEGRAM_URL)
    expect(buildTelegramUrl(TEST_ACCESS_MESSAGE)).toContain(encodeURIComponent(TEST_ACCESS_MESSAGE))

    expect(buildTelegramAppUrl('hello')).toBe(`tg://resolve?domain=${OPERATOR.telegramHandle}&text=hello`)
    expect(buildTelegramAppUrl('   ')).toBe(`tg://resolve?domain=${OPERATOR.telegramHandle}`)
  })

  it('returns null for telegram app URL on the server', () => {
    vi.stubGlobal('window', undefined)
    expect(buildTelegramAppUrl('hello')).toBeNull()
  })

  it('alternates user/assistant roles for prefix-free plain-text lines', () => {
    // kills the mutation index % 2 -> index % 1 (everything would become user)
    expect(parseDemoChat({ id: 5, name: 'alt', demo_chat: 'Первый\nВторой' })).toEqual([
      { role: 'user', content: 'Первый' },
      { role: 'assistant', content: 'Второй' },
    ])
  })

  it('parses plain-text demo with English prefixes', () => {
    // kills: normalized.startsWith('assistant:') -> false and normalized.startsWith('user:') -> false
    expect(parseDemoChat({
      id: 6,
      name: 'eng',
      demo_chat: 'user: hello\nassistant: world',
    })).toEqual([
      { role: 'user', content: 'hello' },
      { role: 'assistant', content: 'world' },
    ])
  })

  it('resolveDemoChat falls back to demo_chat when demoChat is empty', () => {
    // kills: demoChat || demo_chat -> demoChat (an empty string is falsy -> demo_chat is needed)
    expect(resolveDemoChat({ id: 7, name: 'f', demoChat: '', demo_chat: 'fallback text' })).toBe('fallback text')
    expect(resolveDemoChat({ id: 8, name: 'g', demo_chat: 'only snake' })).toBe('only snake')
  })

  it('shuffles without mutating the input and uses Fisher-Yates swap indexes', () => {
    const original = ['a', 'b', 'c']
    vi.spyOn(Math, 'random')
      .mockReturnValueOnce(0) // i=2 -> swap 2 with 0
      .mockReturnValueOnce(0.99) // i=1 -> swap 1 with 1

    expect(shuffle(original)).toEqual(['c', 'b', 'a'])
    expect(original).toEqual(['a', 'b', 'c'])
  })
})
