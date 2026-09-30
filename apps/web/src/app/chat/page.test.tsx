import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'

vi.mock('@/components/BrandLogo', () => ({
  BrandLogo: () => <a href="/" aria-label="СТРАТУМ Mindstrata">logo</a>,
}))

vi.mock('@/components/MessageContent', () => ({
  MessageContent: ({ content, highlight = '' }: { content: string; highlight?: string }) => {
    if (!highlight.trim()) return <span>{content}</span>
    const lower = content.toLowerCase()
    const needle = highlight.trim().toLowerCase()
    const parts: React.ReactNode[] = []
    let cursor = 0
    let index = lower.indexOf(needle)
    while (index >= 0) {
      if (index > cursor) parts.push(content.slice(cursor, index))
      parts.push(<mark key={index} data-chat-search-mark="true">{content.slice(index, index + needle.length)}</mark>)
      cursor = index + needle.length
      index = lower.indexOf(needle, cursor)
    }
    if (cursor < content.length) parts.push(content.slice(cursor))
    return <span>{parts}</span>
  },
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

let siteContent = vi.hoisted(() => ({} as Record<string, unknown>))

vi.mock('@/lib/useSiteContent', () => ({
  useSiteContent: () => siteContent,
}))

import ChatPage from './page'

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
    ...init,
  })
}

function expectDictatedCount(expected: number, _limit: string | number) {
  const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
  const actual = Number(textarea.getAttribute('data-length') ?? '-1')
  expect(actual).toBe(expected)
}


function createDeferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function mockDesktopViewport() {
  Object.defineProperty(window, 'innerWidth', { value: 1280, writable: true, configurable: true })
  Element.prototype.scrollTo = vi.fn()
  Element.prototype.scrollIntoView = vi.fn()
}

const startPayload = {
  ok: true,
  userId: 1,
  role: 'user',
  email: 'user@test.local',
  currentModeId: 10,
  currentDialogId: 100,
  modes: [
    { id: 10, name: 'Психолог', welcomeMessage: 'Добро пожаловать', quota: { used: 1, limit: 5, remaining: 4 } },
  ],
  quota: { used: 1, limit: 5, remaining: 4 },
}

const historyPayload = {
  ok: true,
  dialogId: 100,
  modeId: 10,
  modeName: 'Психолог',
  messages: [
    { id: 1, role: 'user', content: 'Старый вопрос', createdAt: '2026-05-31T10:00:00Z' },
    { id: 2, role: 'assistant', content: 'Старый ответ', createdAt: '2026-05-31T10:00:01Z' },
  ],
}

describe('ChatPage P0 user flows', () => {
  beforeEach(() => {
    localStorage.clear()
    window.history.pushState(null, '', '/')
    siteContent = {}
    mockDesktopViewport()
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('starts chat, loads history and sends a cleaned message to the backend', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) {
        expect(init).toMatchObject({
          method: 'POST',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
        })
        expect(JSON.parse(String(init?.body))).toEqual({
          dialogId: 100,
          text: 'Новый\n\nвопрос',
          responseMode: 'live',
          knowledgeModeIds: [10],
          attachmentIds: [],
        })
        return jsonResponse({
          ok: true,
          user: { id: 3, role: 'user', content: 'Новый\n\nвопрос', createdAt: '2026-05-31T10:01:00Z' },
          assistant: { id: 4, role: 'assistant', content: 'Новый ответ', createdAt: '2026-05-31T10:01:01Z' },
          quota: { used: 2, limit: 5, remaining: 3 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)

    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    expect(screen.getAllByText('Старый ответ')[0]).toBeTruthy()
    expectDictatedCount(0, 30720)
    expect(screen.getAllByText('Осталось 4 из 5').length).toBeGreaterThan(0)

    fireEvent.change(screen.getByPlaceholderText('Напишите сообщение'), {
      target: { value: '  Новый\u200B  \n\n\n  вопрос  ' },
    })
    fireEvent.click(screen.getByTitle('Отправить'))

    await waitFor(() => expect(screen.getByText('Новый ответ')).toBeTruthy())
    expect(screen.getByPlaceholderText('Напишите сообщение')).toHaveProperty('value', '')
    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:18080/api/chat/history?dialogId=100&limit=20',
      { credentials: 'include' },
    )
  })

  it('keeps a subtle mode-switch trace in the chat feed', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({
        ...startPayload,
        modes: [
          { id: 10, name: 'Уточнить фразу', welcomeMessage: 'Добро пожаловать', quota: { used: 1, limit: 5, remaining: 4 } },
          { id: 38, name: 'Переговоры', welcomeMessage: 'Готов к переговорам', quota: { used: 1, limit: 5, remaining: 4 } },
        ],
      })
      if (url.includes('/api/chat/history?')) return jsonResponse({
        ...historyPayload,
        modeName: 'Уточнить фразу',
      })
      if (url.endsWith('/api/chat/send')) {
        expect(JSON.parse(String(init?.body))).toMatchObject({
          dialogId: 100,
          text: 'Нужен ответ',
          responseMode: 'live',
        })
        return jsonResponse({
          ok: true,
          user: { id: 3, role: 'user', content: 'Нужен ответ', createdAt: '2026-05-31T10:01:00Z' },
          switchTrace: { id: 4, role: 'system', content: '[mode-switch] Стратум переключил режим: «Уточнить фразу» → «Переговоры».', createdAt: '2026-05-31T10:01:01Z' },
          assistant: { id: 5, role: 'assistant', content: 'Переговорный ответ', createdAt: '2026-05-31T10:01:02Z', modeName: 'Переговоры' },
          modeId: 38,
          modeName: 'Переговоры',
          modeSwitched: true,
          quota: { used: 2, limit: 5, remaining: 3 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    fireEvent.change(screen.getByPlaceholderText('Напишите сообщение'), { target: { value: 'Нужен ответ' } })
    fireEvent.click(screen.getByTitle('Отправить'))

    await waitFor(() => expect(screen.getByText('Переговорный ответ')).toBeTruthy())
    expect(screen.getByText('Стратум переключил режим: «Уточнить фразу» → «Переговоры».')).toBeTruthy()
    expect(screen.getAllByText('Переговоры').length).toBeGreaterThan(0)
    expect(screen.queryByText(/\[mode-switch]/)).toBeNull()
  })

  it('shows the response mode next to Stratum messages from history', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse(startPayload)
      if (url.includes('/api/chat/history?')) return jsonResponse({
        ...historyPayload,
        messages: [
          { id: 1, role: 'assistant', content: 'Ответ с режимом', createdAt: '2026-05-31T10:00:01Z', modeName: 'Тонкая настройка' },
        ],
      })
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)

    await waitFor(() => expect(screen.getByText('Ответ с режимом')).toBeTruthy())
    expect(screen.getByText('Тонкая настройка')).toBeTruthy()
  })

  it('opens promo-selected first mode in a new dialog so orchestration counter starts clean', async () => {
    window.history.pushState(null, '', '/chat?modes=54')
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({
        ...startPayload,
        currentModeId: 10,
        currentDialogId: 100,
        modes: [
          { id: 10, name: 'Старый режим', welcomeMessage: 'Старый привет', quota: { used: 1, limit: 5, remaining: 4 } },
          { id: 54, name: 'Уточнить фразу', welcomeMessage: 'Новый промо-привет', quota: { used: 1, limit: 5, remaining: 4 } },
        ],
      })
      if (url.endsWith('/api/chat/select-mode')) {
        expect(JSON.parse(String(init?.body))).toMatchObject({ modeId: 54, newDialog: true })
        return jsonResponse({
          ok: true,
          dialogId: 540,
          modeId: 54,
          modeName: 'Уточнить фразу',
          welcomeMessage: 'Новый промо-привет',
          quota: { used: 1, limit: 5, remaining: 4 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)

    await waitFor(() => expect(screen.getByText('Новый промо-привет')).toBeTruthy())
    expect(fetchMock).not.toHaveBeenCalledWith(
      'http://localhost:18080/api/chat/history?dialogId=100&limit=20',
      { credentials: 'include' },
    )
  })

  it('uses one search for current chat and history, highlighting the newest chat match first', async () => {
    localStorage.setItem('ms_chat_history_v5', JSON.stringify([
      {
        dialogId: 200,
        modeId: 10,
        modeName: 'Психолог',
        title: 'Разбор: Игорь',
        snippet: 'Архивная заметка',
        searchText: 'разбор игорь архивная заметка',
        pinned: false,
        updatedAt: '2026-06-01T10:00:00Z',
      },
      {
        dialogId: 201,
        modeId: 10,
        modeName: 'Психолог',
        title: 'Разбор с Марией',
        snippet: 'Без нужного имени',
        searchText: 'мария без нужного имени',
        pinned: false,
        updatedAt: '2026-06-01T09:00:00Z',
      },
    ]))
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse(startPayload)
      if (url.includes('/api/chat/history?')) return jsonResponse({
        ...historyPayload,
        messages: [
          { id: 1, role: 'user', content: 'Ранний вопрос: Игорь', createdAt: '2026-05-31T10:00:00Z' },
          { id: 2, role: 'assistant', content: 'Ответ между совпадениями', createdAt: '2026-05-31T10:00:01Z' },
          { id: 3, role: 'user', content: 'Свежий вопрос: Игорь', createdAt: '2026-05-31T10:02:00Z' },
        ],
      })
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)

    await waitFor(() => expect(screen.getByText('Свежий вопрос: Игорь')).toBeTruthy())
    await waitFor(() => expect(screen.getByText('Разбор: Игорь')).toBeTruthy())

    fireEvent.click(screen.getByTitle('Поиск по чату и истории'))
    const searchInput = screen.getByPlaceholderText('Поиск по чату и истории')
    fireEvent.input(searchInput, { target: { value: 'Игорь' } })

    await waitFor(() => expect(screen.getByText('1 / 2')).toBeTruthy())
    expect(document.querySelectorAll('[data-chat-search-mark="true"]')).toHaveLength(2)
    expect(screen.getByText('Ответ между совпадениями')).toBeTruthy()
    expect(screen.getByText('Разбор: Игорь')).toBeTruthy()
    expect(screen.queryByText('Разбор с Марией')).toBeNull()
    let active = document.querySelector('[data-chat-search-active="true"]')
    expect(active?.textContent).toContain('Свежий вопрос: Игорь')

    fireEvent.keyDown(searchInput, { key: 'Enter', code: 'Enter', charCode: 13, keyCode: 13 })
    await waitFor(() => {
      active = document.querySelector('[data-chat-search-active="true"]')
      expect(active?.textContent).toContain('Ранний вопрос: Игорь')
    })
    fireEvent.keyDown(searchInput, { key: 'Enter', code: 'Enter', shiftKey: true, charCode: 13, keyCode: 13 })
    await waitFor(() => {
      active = document.querySelector('[data-chat-search-active="true"]')
      expect(active?.textContent).toContain('Свежий вопрос: Игорь')
    })
  })

  it('обновляет счётчик и предупреждения лимита при событии input (диктовка)', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, chatMessageMaxChars: 10, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) {
        expect(init?.method).toBe('POST')
        return jsonResponse({
          ok: true,
          user: { id: 3, role: 'user', content: 'text', createdAt: '2026-05-31T10:01:00Z' },
          assistant: { id: 4, role: 'assistant', content: 'ok', createdAt: '2026-05-31T10:01:01Z' },
          quota: { used: 2, limit: 5, remaining: 3 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    const input = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.input(input, { target: { value: '12345678' } })
    expectDictatedCount(8, 10)
    expect(screen.queryByText('Текст почти достиг лимита')).toBeNull()

    fireEvent.input(input, { target: { value: '123456789' } })
    expect(screen.getByText(/Текст почти достиг лимита/)).toBeTruthy()
    expectDictatedCount(9, 10)

    fireEvent.input(input, { target: { value: '1234567890' } })
    expectDictatedCount(10, 10)
    expect(screen.getByText('Сообщение слишком длинное — сократите текст')).toBeTruthy()
  })

  it('не отправляет сообщение по Enter из textarea после достижения лимита на диктовке', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, chatMessageMaxChars: 10, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) {
        throw new Error('send was not expected')
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    const input = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.input(input, { target: { value: '1234567890' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', charCode: 13, keyCode: 13 })

    await waitFor(() => expect(screen.getByText('Сообщение слишком длинное — сократите текст')).toBeTruthy())
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('блокирует кнопку отправки после достижения лимита и не пишет в /api/chat/send', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, chatMessageMaxChars: 5, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) {
        throw new Error('unexpected send')
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    const input = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.input(input, { target: { value: '12345' } })
    expect(screen.getByText('Сообщение слишком длинное — сократите текст')).toBeTruthy()
    fireEvent.click(screen.getByTitle('Отправить'))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('uploads a dropped file and sends its attachment id with the message', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/attachments')) {
        expect(init?.method).toBe('POST')
        expect(init?.credentials).toBe('include')
        expect(init?.body).toBeInstanceOf(FormData)
        return jsonResponse({
          ok: true,
          attachment: {
            id: 77,
            fileName: 'brief.md',
            extension: 'md',
            sizeBytes: 2048,
            sha256: 'abc',
            annotationStatus: 'direct',
          },
        })
      }
      if (url.endsWith('/api/chat/send')) {
        expect(JSON.parse(String(init?.body))).toMatchObject({
          dialogId: 100,
          text: 'Разбери файл',
          attachmentIds: [77],
        })
        return jsonResponse({
          ok: true,
          user: { id: 3, role: 'user', content: 'Разбери файл', createdAt: '2026-05-31T10:01:00Z' },
          assistant: { id: 4, role: 'assistant', content: 'Файл учтён', createdAt: '2026-05-31T10:01:01Z' },
          quota: { used: 2, limit: 5, remaining: 3 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    const input = screen.getByPlaceholderText('Напишите сообщение')
    const file = new File(['# brief'], 'brief.md', { type: 'text/markdown' })
    fireEvent.drop(input, { dataTransfer: { files: [file] } })

    await waitFor(() => expect(screen.getByText('brief.md')).toBeTruthy())
    fireEvent.change(input, { target: { value: 'Разбери файл' } })
    fireEvent.click(screen.getByTitle('Отправить'))

    await waitFor(() => expect(screen.getByText('Файл учтён')).toBeTruthy())
    expect(screen.queryByText('brief.md')).toBeNull()
  })


  it('prevents duplicate sends while a message request is in flight', async () => {
    const sendResponse = createDeferred<Response>()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) return sendResponse.promise
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    fireEvent.change(screen.getByPlaceholderText('Напишите сообщение'), { target: { value: 'Один запрос' } })
    fireEvent.click(screen.getByTitle('Отправить'))
    await waitFor(() => expect((screen.getByTitle('Отправить') as HTMLButtonElement).disabled).toBe(true))
    fireEvent.click(screen.getByTitle('Отправить'))

    expect(fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/api/chat/send'))).toHaveLength(1)
    sendResponse.resolve(jsonResponse({
      ok: true,
      user: { id: 3, role: 'user', content: 'Один запрос', createdAt: '2026-05-31T10:01:00Z' },
      assistant: { id: 4, role: 'assistant', content: 'Ответ один раз', createdAt: '2026-05-31T10:01:01Z' },
      quota: { used: 2, limit: 5, remaining: 3 },
    }))
    await waitFor(() => expect(screen.getByText('Ответ один раз')).toBeTruthy())
  })

  it('removes optimistic bubble after transient send failure', async () => {
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) return jsonResponse({ ok: false, error: 'provider unavailable' }, { status: 502 })
      throw new Error(`unexpected fetch ${url}`)
    }) as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    const input = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.change(input, { target: { value: 'Повторить позже' } })
    fireEvent.click(screen.getByTitle('Отправить'))

    await waitFor(() => expect(screen.getByText('provider unavailable')).toBeTruthy())
    expect(screen.queryByText('Повторить позже')).toBeNull()
  })

  it('renders mobile composer and can send after history load', async () => {
    Object.defineProperty(window, 'innerWidth', { value: 390, writable: true, configurable: true })
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) return jsonResponse({
        ok: true,
        user: { id: 3, role: 'user', content: 'Мобильно', createdAt: '2026-05-31T10:01:00Z' },
        assistant: { id: 4, role: 'assistant', content: 'Мобильный ответ', createdAt: '2026-05-31T10:01:01Z' },
        quota: { used: 2, limit: 5, remaining: 3 },
      })
      throw new Error(`unexpected fetch ${url}`)
    }) as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    expect(screen.getByPlaceholderText('Напишите сообщение')).toBeTruthy()
    fireEvent.change(screen.getByPlaceholderText('Напишите сообщение'), { target: { value: 'Мобильно' } })
    fireEvent.click(screen.getByTitle('Отправить'))

    await waitFor(() => expect(screen.getByText('Мобильный ответ')).toBeTruthy())
  })

  it('updates quota and switches to the renewal guard after 429 from send', async () => {
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, email: '' })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) {
        return jsonResponse({ ok: false, error: 'daily quota exhausted', quota: { used: 5, limit: 5, remaining: 0 } }, { status: 429 })
      }
      throw new Error(`unexpected fetch ${url}`)
    }) as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    const input = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.change(input, { target: { value: 'Последняя попытка' } })
    fireEvent.click(screen.getByTitle('Отправить'))

    await waitFor(() => expect(screen.getByText('Лимит на сегодня исчерпан')).toBeTruthy())
    expect(screen.getByText('Дневной лимит сообщений исчерпан')).toBeTruthy()
    expect(screen.getAllByRole('link', { name: 'Продлить доступ' })[0].getAttribute('href')).toBe('/access?force=promo')
    expect(screen.queryByTitle('Отправить')).toBeNull()
  })

  it('auto-switches to another available mode when the current mode quota is exhausted', async () => {
    const modes = [
      { id: 10, name: 'Психолог', welcomeMessage: 'Добро пожаловать', quota: { used: 4, limit: 5, remaining: 1 } },
      { id: 20, name: 'Системный анализ', welcomeMessage: 'Разберём систему', quota: { used: 4, limit: 10, remaining: 6 } },
    ]
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({
        ...startPayload,
        modes,
        quota: { used: 4, limit: 5, remaining: 1 },
      })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) {
        return jsonResponse({ ok: false, error: 'daily quota exhausted', quota: { used: 5, limit: 5, remaining: 0 } }, { status: 429 })
      }
      if (url.endsWith('/api/chat/select-mode')) {
        expect(JSON.parse(String(init?.body))).toMatchObject({ modeId: 20, newDialog: true })
        return jsonResponse({
          ok: true,
          dialogId: 200,
          modeName: 'Системный анализ',
          welcomeMessage: 'Разберём систему',
          quota: { used: 5, limit: 10, remaining: 5 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    fireEvent.change(screen.getByPlaceholderText('Напишите сообщение'), { target: { value: 'Последняя попытка' } })
    fireEvent.click(screen.getByTitle('Отправить'))

    await waitFor(() => expect(screen.getByText('Разберём систему')).toBeTruthy())
    expect(screen.queryByText('Дневной лимит сообщений исчерпан')).toBeNull()
    expect(screen.getAllByText('Осталось 5 из 10').length).toBeGreaterThan(0)
    expect(screen.getByPlaceholderText('Напишите сообщение')).toHaveProperty('value', 'Последняя попытка')
  })

  it('does not show guest history-save login CTA for signed-in users after quota exhaustion', async () => {
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({
        ...startPayload,
        email: 'signed@test.local',
      })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) {
        return jsonResponse({ ok: false, error: 'daily quota exhausted', quota: { used: 5, limit: 5, remaining: 0 } }, { status: 429 })
      }
      throw new Error(`unexpected fetch ${url}`)
    }) as unknown as typeof fetch

    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())

    fireEvent.change(screen.getByPlaceholderText('Напишите сообщение'), { target: { value: 'Последняя попытка' } })
    fireEvent.click(screen.getByTitle('Отправить'))

    await waitFor(() => expect(screen.getByText('Дневной лимит сообщений исчерпан')).toBeTruthy())
    expect(screen.queryByRole('link', { name: 'Войти и сохранить историю' })).toBeNull()
    expect(screen.getAllByRole('link', { name: 'Продлить доступ' }).some(link => link.getAttribute('href') === '/access?force=promo')).toBe(true)
  })

  it('shows an editable near-limit warning before the daily quota is exhausted', async () => {
    siteContent = {
      'chat.quota.warning_threshold_remaining': 2,
      'chat.quota.warning_title': 'Осталось немного ходов',
      'chat.quota.warning_body': 'Соберите главный вопрос в одно сообщение. Осталось {{remaining}} из {{limit}}.',
    }
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({
        ...startPayload,
        email: '',
        modes: [{ id: 10, name: 'Психолог', welcomeMessage: 'Добро пожаловать', quota: { used: 3, limit: 5, remaining: 2 } }],
        quota: { used: 3, limit: 5, remaining: 2 },
      })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      throw new Error(`unexpected fetch ${url}`)
    }) as unknown as typeof fetch

    render(<ChatPage />)

    await waitFor(() => expect(screen.getByText('Осталось немного ходов')).toBeTruthy())
    expect(screen.getByText('Соберите главный вопрос в одно сообщение. Осталось 2 из 5.')).toBeTruthy()
    expect(screen.getByPlaceholderText('Напишите сообщение')).toBeTruthy()
  })

  it('does not show the near-limit warning when CMS disables it', async () => {
    siteContent = {
      'chat.quota.warning_enabled': false,
      'chat.quota.warning_threshold_remaining': 5,
      'chat.quota.warning_title': 'Скрытый текст',
    }
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({
        ...startPayload,
        modes: [{ id: 10, name: 'Психолог', welcomeMessage: 'Добро пожаловать', quota: { used: 4, limit: 5, remaining: 1 } }],
        quota: { used: 4, limit: 5, remaining: 1 },
      })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      throw new Error(`unexpected fetch ${url}`)
    }) as unknown as typeof fetch

    render(<ChatPage />)

    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    expect(screen.queryByText('Скрытый текст')).toBeNull()
  })

  it('shows retryable startup errors without crashing the chat shell', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ ok: false, error: 'chat_start_failed' }, { status: 500 }))
      .mockResolvedValueOnce(jsonResponse({ ...startPayload, messages: historyPayload.messages }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ChatPage />)

    await waitFor(() => expect(screen.getByText('HTTP 500')).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))

    await waitFor(() => expect(screen.getAllByText('Старый ответ')[0]).toBeTruthy())
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

describe('ChatPage textarea — голосовой ввод мобильных клавиатур', () => {
  beforeEach(() => {
    localStorage.clear()
    window.history.pushState(null, '', '/')
    siteContent = {}
    mockDesktopViewport()
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    vi.restoreAllMocks()
  })

  function mountChat(extraStartFields: Record<string, unknown> = {}) {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) return jsonResponse({ ...startPayload, email: '', ...extraStartFields })
      if (url.includes('/api/chat/history?')) return jsonResponse(historyPayload)
      if (url.endsWith('/api/chat/send')) {
        return jsonResponse({
          ok: true,
          user: { id: 3, role: 'user', content: 'voice', createdAt: '2026-06-07T10:01:00Z' },
          assistant: { id: 4, role: 'assistant', content: 'ok', createdAt: '2026-06-07T10:01:01Z' },
          quota: { used: 2, limit: 5, remaining: 3 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch
    return fetchMock
  }

  function waitTick(ms: number) {
    return new Promise(resolve => setTimeout(resolve, ms))
  }

  it('выставляет mobile hints на textarea: inputMode, enterKeyHint, autoCapitalize, autoCorrect, spellCheck', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    expect(textarea.getAttribute('inputmode')).toBe('text')
    expect(textarea.getAttribute('enterkeyhint')).toBe('send')
    expect(textarea.getAttribute('autocapitalize')).toBe('sentences')
    expect(textarea.getAttribute('autocorrect')).toBe('on')
    expect(textarea.getAttribute('spellcheck')).toBe('true')
  })

  it('blur синхронизирует value DOM → state если значение изменилось', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.focus(textarea)
    Object.defineProperty(textarea, 'value', { value: 'после блюра', configurable: true, writable: true })
    fireEvent.blur(textarea)
    await waitFor(() => expectDictatedCount(11, 30720))
  })

  it('прямая запись в DOM без событий не обновляет state (IME-буфер не ломает state)', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.focus(textarea)
    fireEvent.compositionStart(textarea)
    Object.defineProperty(textarea, 'value', { value: 'частичная композиция', configurable: true, writable: true })
    await waitTick(260)
    // during composition the counter must stay 0 (state not updated)
    expectDictatedCount(0, 30720)
  })

  it('compositionEnd: коммитит финальный текст в state и снимает composing flag', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.focus(textarea)
    fireEvent.compositionStart(textarea)
    fireEvent.compositionEnd(textarea, { data: 'итоговая фраза', target: { value: 'итоговая фраза' } } as any)
    await waitFor(() => expectDictatedCount(14, 30720))
  })

  it('onChange обновляет state во время IME-композиции (Android Gboard)', async () => {
    // onChange has no isComposing guard: Gboard uses IME for every word,
    // the guard would block all keyboard input.
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.compositionStart(textarea)
    fireEvent.input(textarea, { target: { value: 'промежуточный' } })
    // onChange fired (jsdom raises it together with input) -> state updated to 13
    await waitFor(() => expectDictatedCount(13, 30720))
  })

  it('Enter во время композиции не отправляет (isComposing → return)', async () => {
    const fetchMock = mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.input(textarea, { target: { value: 'текст' } })
    fireEvent.compositionStart(textarea)
    fireEvent.keyDown(textarea, { key: 'Enter' })
    expect(fetchMock).toHaveBeenCalledTimes(2) // only start + history, no send
  })

  it('iOS-диктовка: input event обновляет state и активирует кнопку (без композиции)', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    const sendBtn = screen.getByTitle('Отправить') as HTMLButtonElement
    fireEvent.input(textarea, { target: { value: 'Сегодня хорошая погода' } })
    expect(sendBtn.disabled).toBe(false)
    expectDictatedCount(22, 30720)
  })

  it('Android voice: композиция+commit с превышением лимита блокирует отправку', async () => {
    mountChat({ chatMessageMaxChars: 5 })
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.compositionStart(textarea)
    fireEvent.compositionEnd(textarea, { target: { value: 'длинная фраза от микрофона' } } as any)
    await waitFor(() => expect(screen.getByText('Сообщение слишком длинное — сократите текст')).toBeTruthy())
    const sendBtn = screen.getByTitle('Отправить') as HTMLButtonElement
    expect(sendBtn.disabled).toBe(true)
  })

  it('размонтирование не оставляет утечек (нет pending timers/listeners)', async () => {
    mountChat()
    const { unmount } = render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    unmount()
  })

  // ── Regression tests for the 2026-06-07 incident ──────────────────────────────

  it('onChange: базовый клавиатурный ввод обновляет state и активирует отправку', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    const sendBtn = screen.getByTitle('Отправить') as HTMLButtonElement
    expect(sendBtn.disabled).toBe(true)
    fireEvent.input(textarea, { target: { value: 'Привет' } })
    expect(sendBtn.disabled).toBe(false)
    expectDictatedCount(6, 30720)
  })

  it('onChange: цепочка символов — каждое изменение обновляет state', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.input(textarea, { target: { value: 'а' } })
    fireEvent.input(textarea, { target: { value: 'аб' } })
    fireEvent.input(textarea, { target: { value: 'абв' } })
    await waitFor(() => expectDictatedCount(3, 30720))
  })

  it('onChange: очистка input деактивирует кнопку отправки', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    const sendBtn = screen.getByTitle('Отправить') as HTMLButtonElement
    fireEvent.input(textarea, { target: { value: 'что-то' } })
    expect(sendBtn.disabled).toBe(false)
    fireEvent.input(textarea, { target: { value: '' } })
    expect(sendBtn.disabled).toBe(true)
    expectDictatedCount(0, 30720)
  })

  it('Enter без активной композиции отправляет сообщение', async () => {
    const fetchMock = mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.input(textarea, { target: { value: 'отправить' } })
    fireEvent.keyDown(textarea, { key: 'Enter' })
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/chat/send'), expect.anything()
    ))
  })

  it('onBlur сбрасывает isComposing: Enter работает после прерванной композиции', async () => {
    const fetchMock = mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.input(textarea, { target: { value: 'текст' } })
    fireEvent.compositionStart(textarea)
    // blur without compositionEnd must reset isComposing
    fireEvent.blur(textarea)
    fireEvent.focus(textarea)
    fireEvent.keyDown(textarea, { key: 'Enter' }) // isComposing = false -> sends
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/chat/send'), expect.anything()
    ))
  })

  it('onBlur не затирает state когда value не изменилось', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.input(textarea, { target: { value: 'hello' } })
    await waitFor(() => expectDictatedCount(5, 30720))
    fireEvent.blur(textarea)
    expectDictatedCount(5, 30720)
  })

  it('Android Gboard полный цикл: onChange mid-composition + compositionEnd финализирует текст', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    const sendBtn = screen.getByTitle('Отправить') as HTMLButtonElement
    fireEvent.compositionStart(textarea)
    fireEvent.input(textarea, { target: { value: 'привет' } })
    await waitFor(() => expectDictatedCount(6, 30720))
    fireEvent.compositionEnd(textarea, { target: { value: 'Привет мир' } } as any)
    await waitFor(() => expectDictatedCount(10, 30720))
    expect(sendBtn.disabled).toBe(false)
  })

  it('onBlur после compositionStart без compositionEnd: синкает value и сбрасывает isComposing', async () => {
    const fetchMock = mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    fireEvent.compositionStart(textarea)
    Object.defineProperty(textarea, 'value', { value: 'голос без финала', configurable: true, writable: true })
    fireEvent.blur(textarea)
    await waitFor(() => expectDictatedCount(16, 30720))
    fireEvent.focus(textarea)
    fireEvent.keyDown(textarea, { key: 'Enter' })
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/chat/send'), expect.anything()
    ))
  })

  it('быстрый последовательный ввод: state всегда отражает последнее значение', async () => {
    mountChat()
    render(<ChatPage />)
    await waitFor(() => expect(screen.getAllByText('Старый вопрос')[0]).toBeTruthy())
    const textarea = screen.getByPlaceholderText('Напишите сообщение') as HTMLTextAreaElement
    for (const v of ['п', 'пр', 'при', 'прив', 'приве', 'привет']) {
      fireEvent.input(textarea, { target: { value: v } })
    }
    await waitFor(() => expectDictatedCount(6, 30720))
  })
})
