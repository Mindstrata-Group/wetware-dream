import { afterEach, describe, expect, it, vi } from 'vitest'
import { createChatApiActions } from './chatApiActions'
import type { ChatApiActionsContext } from './chatActionTypes'

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
    ...init,
  })
}

function createContext(overrides: Partial<ChatApiActionsContext> = {}): ChatApiActionsContext {
  const base: ChatApiActionsContext = {
    modes: [{ id: 10, name: 'Психолог', quota: { used: 1, limit: 5, remaining: 4 } }],
    quota: { used: 1, limit: 5, remaining: 4 },
    modeId: 10,
    modeName: 'Психолог',
    input: '',
    chatMessageMaxChars: 30720,
    attachments: [],
    dialogId: 100,
    selectedModeIds: [10],
    beforeId: null,
    responseMode: 'live',
    dialogCompleted: false,
    dialogAccessRequired: false,
    dailyLimitExhausted: false,
    sending: false,
    completing: false,
    startingAfterSummary: false,
    modeSelectionLoadedRef: { current: false },
    loadingMoreRef: { current: false },
    setLoading: vi.fn(),
    setError: vi.fn(),
    setErrorDebug: vi.fn(),
    setUserRole: vi.fn(),
    setUserEmail: vi.fn(),
    setResponseMode: vi.fn(),
    setModes: vi.fn(),
    setSelectedModeIds: vi.fn(),
    setPromoModeIds: vi.fn(),
    setQuota: vi.fn(),
    setChatMessageMaxChars: vi.fn(),
    setModeId: vi.fn(),
    setModeName: vi.fn(),
    setDialogId: vi.fn(),
    setMessages: vi.fn(),
    setBeforeId: vi.fn(),
    setSummaryHistoryVisible: vi.fn(),
    setDialogAccessRequired: vi.fn(),
    setDialogCompleted: vi.fn(),
    setInput: vi.fn(),
    setAttachments: vi.fn(),
    setOrchestrationNotice: vi.fn(),
    setLockedModeNotice: vi.fn(),
    setSending: vi.fn(),
    setCompleting: vi.fn(),
    setStartingAfterSummary: vi.fn(),
    scrollChatToBottom: vi.fn(),
    setMaxLinked: vi.fn(),
    setMaxBotLink: vi.fn(),
  }
  return { ...base, ...overrides }
}

afterEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('createChatApiActions', () => {
  it('применяет лимит символов из /api/chat/start в state', async () => {
    const setChatMessageMaxChars = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) {
        return jsonResponse({
          ok: true,
          userId: 1,
          role: 'user',
          currentModeId: 10,
          modes: [{ id: 10, name: 'Психолог', welcomeMessage: 'Добро пожаловать' }],
          quota: { used: 1, limit: 5, remaining: 4 },
          chatMessageMaxChars: 12345,
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch
    const apiActions = createChatApiActions(createContext({ setChatMessageMaxChars }))

    await apiActions.start()

    expect(setChatMessageMaxChars).toHaveBeenCalledWith(12345)
  })

  it('не обновляет лимит, если /api/chat/start его не вернул', async () => {
    const setChatMessageMaxChars = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) {
        return jsonResponse({
          ok: true,
          userId: 1,
          role: 'user',
          currentModeId: 10,
          modes: [{ id: 10, name: 'Психолог', welcomeMessage: 'Добро пожаловать' }],
          quota: { used: 1, limit: 5, remaining: 4 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch
    const apiActions = createChatApiActions(createContext({ setChatMessageMaxChars }))

    await apiActions.start()

    expect(setChatMessageMaxChars).not.toHaveBeenCalled()
  })

  it('для обычного промо-пользователя не сужает стартовый выбор до одного сохраненного режима', async () => {
    localStorage.setItem('ms_chat_last_selected_mode_ids_v1', JSON.stringify([10]))
    const setSelectedModeIds = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/chat/start')) {
        return jsonResponse({
          ok: true,
          userId: 1,
          role: 'user',
          currentModeId: 10,
          modes: [
            { id: 10, name: 'Найти причины проблемы', quota: { used: 1, limit: 5, remaining: 4 } },
            { id: 54, name: 'Уточнить фразу', quota: { used: 1, limit: 5, remaining: 4 } },
          ],
          quota: { used: 1, limit: 5, remaining: 4 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await createChatApiActions(createContext({ setSelectedModeIds })).start()

    expect(setSelectedModeIds).toHaveBeenCalledWith([10, 54])
  })

  it('блокирует отправку в sendMessage при превышении лимита и пишет ошибку', async () => {
    const setError = vi.fn()
    const fetchMock = vi.fn()
    globalThis.fetch = fetchMock as unknown as typeof fetch
    const apiActions = createChatApiActions(createContext({
      chatMessageMaxChars: 3,
      input: 'превышение',
      setError,
      setSending: vi.fn(),
      setMessages: vi.fn(),
      setInput: vi.fn(),
      setSummaryHistoryVisible: vi.fn(),
      setBeforeId: vi.fn(),
    }))

    await apiActions.sendMessage('превышение')

    expect(setError).toHaveBeenCalledTimes(1)
    expect(setError).toHaveBeenCalledWith(expect.stringContaining('максимум 3'))
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('очищает текст перед отправкой и пишет очищенный текст в /api/chat/send', async () => {
    const setMessages = vi.fn()
    const setInput = vi.fn()
    const setSummaryHistoryVisible = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/send')) {
        expect(init?.method).toBe('POST')
        expect(init?.credentials).toBe('include')
        expect(JSON.parse(String(init?.body))).toEqual({
          dialogId: 100,
          text: 'Новый\n\nвопрос',
          responseMode: 'live',
          knowledgeModeIds: [10],
          attachmentIds: [55],
        })
        return jsonResponse({
          ok: true,
          user: { id: 3, role: 'user', content: 'Новый\n\nвопрос', createdAt: '2026-05-31T10:01:00Z' },
          assistant: { id: 4, role: 'assistant', content: 'Ответ', createdAt: '2026-05-31T10:01:01Z' },
          quota: { used: 2, limit: 5, remaining: 3 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const apiActions = createChatApiActions(createContext({
      input: '  Новый\n\n\n  вопрос  ',
      attachments: [{ id: 55, fileName: 'x', extension: 'txt', sizeBytes: 1, sha256: 'x', annotationStatus: 'direct' }, { id: -1, fileName: 'y', extension: 'txt', sizeBytes: 1, sha256: 'y', annotationStatus: 'direct' }],
      setMessages,
      setInput,
      setSummaryHistoryVisible,
    }))

    await apiActions.sendMessage()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(setMessages).toHaveBeenCalled()
    expect(setInput).toHaveBeenCalledWith('')
    expect(setSummaryHistoryVisible).toHaveBeenCalledWith(false)
  })

  it('отправляет служебный текст для отправки вложений без текста', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/chat/send')) {
        expect(JSON.parse(String(init?.body))).toEqual({
          dialogId: 100,
          text: 'Проанализируй приложенные файлы.',
          responseMode: 'live',
          knowledgeModeIds: [10],
          attachmentIds: [55],
        })
        return jsonResponse({
          ok: true,
          user: { id: 3, role: 'user', content: 'Проанализируй приложенные файлы.', createdAt: '2026-05-31T10:01:00Z' },
          assistant: { id: 4, role: 'assistant', content: 'Ответ', createdAt: '2026-05-31T10:01:01Z' },
          quota: { used: 2, limit: 5, remaining: 3 },
        })
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const apiActions = createChatApiActions(createContext({
      input: '   ',
      attachments: [{ id: 55, fileName: 'x', extension: 'txt', sizeBytes: 1, sha256: 'x', annotationStatus: 'direct' }],
      setMessages: vi.fn(),
      setInput: vi.fn(),
      setSummaryHistoryVisible: vi.fn(),
    }))

    await apiActions.sendMessage()

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('не отправляет ничего при пустом чистом тексте и без вложений', async () => {
    const fetchMock = vi.fn()
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const apiActions = createChatApiActions(createContext({
      input: '   ',
      setMessages: vi.fn(),
      setInput: vi.fn(),
      setSummaryHistoryVisible: vi.fn(),
      setSending: vi.fn(),
    }))

    await apiActions.sendMessage()

    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('не устанавливает chatMessageMaxChars для Infinity и <= 0 (убивает > 0 и isFinite мутации)', async () => {
    const setChatMessageMaxChars = vi.fn()
    for (const badVal of [Infinity, -Infinity, 0, -1]) {
      setChatMessageMaxChars.mockClear()
      const fetchMock = vi.fn(async () => jsonResponse({
        ok: true, userId: 1, role: 'user', currentModeId: 10, modes: [],
        chatMessageMaxChars: badVal,
      }))
      globalThis.fetch = fetchMock as unknown as typeof fetch
      await createChatApiActions(createContext({ setChatMessageMaxChars })).start()
      expect(setChatMessageMaxChars).not.toHaveBeenCalled()
    }
  })

  it('применяет Math.floor к chatMessageMaxChars (убивает floor → ceil мутацию)', async () => {
    const setChatMessageMaxChars = vi.fn()
    const fetchMock = vi.fn(async () => jsonResponse({
      ok: true, userId: 1, role: 'user', currentModeId: 10, modes: [],
      chatMessageMaxChars: 30720.9,
    }))
    globalThis.fetch = fetchMock as unknown as typeof fetch
    await createChatApiActions(createContext({ setChatMessageMaxChars })).start()
    expect(setChatMessageMaxChars).toHaveBeenCalledWith(30720)
  })

  it('фильтрует вложения с id <= 0 из attachmentIds (убивает id > 0 мутацию)', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith('/api/chat/send')) {
        const body = JSON.parse(String(init?.body))
        expect(body.attachmentIds).toEqual([55])
        return jsonResponse({
          ok: true,
          user: { id: 3, role: 'user', content: 'Файлы', createdAt: '2026-01-01T00:00:00Z' },
          assistant: { id: 4, role: 'assistant', content: 'OK', createdAt: '2026-01-01T00:00:01Z' },
        })
      }
      throw new Error(`unexpected fetch ${String(input)}`)
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch
    const apiActions = createChatApiActions(createContext({
      input: 'Текст',
      attachments: [
        { id: 55, fileName: 'ok.txt', extension: 'txt', sizeBytes: 1, sha256: 'a', annotationStatus: 'direct' },
        { id: 0, fileName: 'bad.txt', extension: 'txt', sizeBytes: 1, sha256: 'b', annotationStatus: 'direct' },
        { id: -1, fileName: 'neg.txt', extension: 'txt', sizeBytes: 1, sha256: 'c', annotationStatus: 'direct' },
      ],
      setMessages: vi.fn(),
      setInput: vi.fn(),
      setSummaryHistoryVisible: vi.fn(),
    }))
    await apiActions.sendMessage()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('не отправляет сообщение при завершённом диалоге и не меняет sending', async () => {
    const fetchMock = vi.fn()
    const setSending = vi.fn()
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const apiActions = createChatApiActions(createContext({
      dialogCompleted: true,
      input: 'текст',
      setMessages: vi.fn(),
      setInput: vi.fn(),
      setSummaryHistoryVisible: vi.fn(),
      setSending,
    }))

    await apiActions.sendMessage()

    expect(fetchMock).not.toHaveBeenCalled()
    expect(setSending).not.toHaveBeenCalled()
  })

  describe('sendMessage — обработка ошибок и откат optimistic-сообщения', () => {
    it('откатывает optimistic-сообщение и пишет ApiError-debug при 500', async () => {
      const setMessages = vi.fn()
      const setError = vi.fn()
      const setErrorDebug = vi.fn()
      const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).endsWith('/api/chat/send')) {
          return jsonResponse({ ok: false, error: 'boom', debug: { hint: 'x' } }, { status: 500 })
        }
        throw new Error(`unexpected fetch ${String(input)}`)
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({
        input: 'привет',
        setMessages,
        setError,
        setErrorDebug,
      }))
      await apiActions.sendMessage()

      // !res.ok (not 429) -> rollback in the if-branch, throw ApiError -> another
      // rollback in the catch block: 3 setMessages calls in total (optimistic + 2
      // rollbacks, both idempotent: they filter an already filtered array).
      expect(setMessages).toHaveBeenCalledTimes(3)
      // optimisticId is generated inside the function as -Date.now(); we extract
      // the real id from the first (optimistic-add) call so the rollback filter
      // really works on it, not on a number that happens to match.
      const addFn = setMessages.mock.calls[0][0]
      const optimisticMsg = addFn([])[0]
      const rollbackFn = setMessages.mock.calls[1][0]
      const afterRollback = rollbackFn([optimisticMsg, { id: 1, role: 'assistant', content: 'старое' }])
      expect(afterRollback).toEqual([{ id: 1, role: 'assistant', content: 'старое' }])
      expect(setError).toHaveBeenCalledWith('boom')
      expect(setErrorDebug).toHaveBeenCalledWith({ hint: 'x' })
    })

    it('на 429 откатывает optimistic-сообщение (до проверки статуса) и восстанавливает input', async () => {
      const setMessages = vi.fn()
      const setInput = vi.fn()
      const setQuota = vi.fn()
      const setModes = vi.fn()
      const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).endsWith('/api/chat/send')) {
          return jsonResponse({ ok: false, error: 'daily_quota_exhausted', quota: { used: 5, limit: 5, remaining: 0 } }, { status: 429 })
        }
        throw new Error(`unexpected fetch ${String(input)}`)
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({
        input: 'важный текст',
        setMessages,
        setInput,
        setQuota,
        setModes,
      }))
      await apiActions.sendMessage()

      expect(setInput).toHaveBeenCalledWith('важный текст')
      expect(setQuota).toHaveBeenCalledWith({ used: 5, limit: 5, remaining: 0 })
      expect(setModes).toHaveBeenCalled()
      // The rollback (setMessages) happens BEFORE the res.status===429 check in
      // the code (shared by all !res.ok branches): optimistic(1) + rollback(2).
      // Kills the mutation that removes the early `return` inside the 429 branch (without
      // the early return the test would catch a third call ApiError->catch->rollback).
      expect(setMessages).toHaveBeenCalledTimes(2)
    })

    it('при json.ok=false (200) откатывает optimistic-сообщение', async () => {
      const setMessages = vi.fn()
      const setError = vi.fn()
      const fetchMock = vi.fn(async () => jsonResponse({ ok: false, error: 'что-то не так' }))
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({
        input: 'привет',
        setMessages,
        setError,
      }))
      await apiActions.sendMessage()

      // json.ok=false with res.ok=true -> rollback in the if(!json.ok) branch,
      // throw ApiError -> another rollback in catch: 3 setMessages calls.
      expect(setMessages).toHaveBeenCalledTimes(3)
      expect(setError).toHaveBeenCalledWith('что-то не так')
    })

    it('строит switchTrace-заглушку, если сервер не прислал switchTrace, но modeSwitched=true', async () => {
      const setMessages = vi.fn()
      const setOrchestrationNotice = vi.fn()
      const setModeId = vi.fn()
      const setModeName = vi.fn()
      const fetchMock = vi.fn(async () => jsonResponse({
        ok: true,
        modeId: 99,
        modeName: 'Новый режим',
        modeSwitched: true,
        user: { id: 3, role: 'user', content: 'x', createdAt: '2026-01-01T00:00:00Z' },
        assistant: { id: 4, role: 'assistant', content: 'y', createdAt: '2026-01-01T00:00:01Z' },
      }))
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({
        input: 'x',
        modeId: 10,
        modeName: 'Старый режим',
        selectedModeIds: [10],
        setMessages,
        setOrchestrationNotice,
        setModeId,
        setModeName,
      }))
      await apiActions.sendMessage()

      expect(setModeId).toHaveBeenCalledWith(99)
      expect(setModeName).toHaveBeenCalledWith('Новый режим')
      expect(setOrchestrationNotice).toHaveBeenCalledWith(expect.stringContaining('Новый режим'))
      const finalCall = setMessages.mock.calls[setMessages.mock.calls.length - 1][0]
      // optimisticId in the code is a real -Date.now(), not -999, so our
      // fake prev message with id=-999 is not cut by the filter (it does not match):
      // it stays as is, plus user/switchTrace/assistant arrive.
      const result = finalCall([{ id: -999, role: 'user', content: 'x' }])
      expect(result).toHaveLength(4) // [remaining prev] + user + system switchTrace + assistant
      expect(result[1].role).toBe('user')
      expect(result[2].role).toBe('system')
      expect(result[3].role).toBe('assistant')
    })
  })

  describe('openMode', () => {
    it('успешно открывает режим: обновляет dialogId/modeName/quota/messages', async () => {
      const setDialogId = vi.fn()
      const setModeName = vi.fn()
      const setDialogAccessRequired = vi.fn()
      const setMessages = vi.fn()
      const setInput = vi.fn()
      const setDialogCompleted = vi.fn()
      const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        expect(String(input)).toContain('/api/chat/select-mode')
        expect(JSON.parse(String(init?.body))).toEqual({ modeId: 20, newDialog: false })
        return jsonResponse({
          ok: true,
          dialogId: 555,
          modeName: 'Новый режим',
          quota: { used: 0, limit: 5, remaining: 5 },
          welcomeMessage: 'Добро пожаловать в режим',
          modeId: 20,
        })
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({
        setDialogId, setModeName, setDialogAccessRequired, setMessages, setInput, setDialogCompleted,
      }))
      await apiActions.openMode(20)

      expect(setDialogId).toHaveBeenCalledWith(555)
      expect(setDialogAccessRequired).toHaveBeenCalledWith(false)
      expect(setModeName).toHaveBeenCalledWith('Новый режим')
      expect(setDialogCompleted).toHaveBeenCalledWith(false)
      const messagesResult = setMessages.mock.calls[0][0]([])
      expect(messagesResult).toHaveLength(1)
      expect(messagesResult[0].content).toBe('Добро пожаловать в режим')
    })

    it('на 429 сохраняет preservedInput и НЕ бросает исключение', async () => {
      const setInput = vi.fn()
      const setQuota = vi.fn()
      const setModes = vi.fn()
      const setError = vi.fn()
      const fetchMock = vi.fn(async () => jsonResponse({ ok: false, quota: { used: 5, limit: 5, remaining: 0 } }, { status: 429 }))
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ setInput, setQuota, setModes, setError }))
      await apiActions.openMode(20, 'сохранённый текст')

      expect(setInput).toHaveBeenCalledWith('сохранённый текст')
      expect(setQuota).toHaveBeenCalledWith({ used: 5, limit: 5, remaining: 0 })
      // openMode unconditionally calls setError(null) at the start (reset before
      // the attempt); that is not a UX error, but a 429 must not add a SECOND call
      // with the real error text (early return before the catch block).
      expect(setError).toHaveBeenCalledTimes(1)
      expect(setError).toHaveBeenCalledWith(null)
    })

    it('при newDialog=true сбрасывает beforeId в null и НЕ склеивает с предыдущими сообщениями', async () => {
      const setBeforeId = vi.fn()
      const setMessages = vi.fn()
      const fetchMock = vi.fn(async () => jsonResponse({
        ok: true, dialogId: 1, modeName: 'M', welcomeMessage: 'Привет', modeId: 20,
      }))
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ setBeforeId, setMessages }))
      await apiActions.openMode(20, '', true)

      expect(setBeforeId).toHaveBeenCalledWith(null)
      const result = setMessages.mock.calls[0][0]([{ id: 1, role: 'user', content: 'старое' }])
      // newDialog=true -> ignores prev entirely, only welcome.
      expect(result).toEqual([{ id: expect.any(Number), role: 'assistant', content: 'Привет', createdAt: expect.any(String), modeId: 20, modeName: 'M' }])
    })

    it('при ошибке сети пишет error через catch', async () => {
      const setError = vi.fn()
      const fetchMock = vi.fn(async () => { throw new Error('network down') })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ setError }))
      await apiActions.openMode(20)

      expect(setError).toHaveBeenCalledWith('network down')
    })
  })

  describe('openBaseMode', () => {
    it('сбрасывает modeId/modeName/quota/messages на базовый режим', async () => {
      const setModeId = vi.fn()
      const setModeName = vi.fn()
      const setQuota = vi.fn()
      const setMessages = vi.fn()
      const setBeforeId = vi.fn()
      const setDialogId = vi.fn()
      const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        expect(JSON.parse(String(init?.body))).toEqual({ modeId: 0, newDialog: false })
        return jsonResponse({ ok: true, dialogId: 777, quota: null })
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({
        setModeId, setModeName, setQuota, setMessages, setBeforeId, setDialogId,
      }))
      await apiActions.openBaseMode()

      // The first call is an optimistic reset BEFORE the server responds.
      expect(setModeId).toHaveBeenCalledWith('')
      expect(setModeName).toHaveBeenNthCalledWith(1, 'Базовый ИИ')
      expect(setDialogId).toHaveBeenCalledWith(777)
      expect(setMessages).toHaveBeenCalledWith([])
      expect(setBeforeId).toHaveBeenCalledWith(null)
    })

    it('при HTTP-ошибке пишет error', async () => {
      const setError = vi.fn()
      const fetchMock = vi.fn(async () => jsonResponse({ error: 'сервер недоступен' }, { status: 500 }))
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ setError }))
      await apiActions.openBaseMode()

      expect(setError).toHaveBeenCalledWith('сервер недоступен')
    })
  })

  describe('completeDialog', () => {
    it('ничего не делает без dialogId/modeId, или если уже completing', async () => {
      const fetchMock = vi.fn()
      globalThis.fetch = fetchMock as unknown as typeof fetch

      await createChatApiActions(createContext({ dialogId: 0 })).completeDialog()
      await createChatApiActions(createContext({ modeId: '' })).completeDialog()
      await createChatApiActions(createContext({ dialogAccessRequired: true })).completeDialog()
      await createChatApiActions(createContext({ completing: true })).completeDialog()

      expect(fetchMock).not.toHaveBeenCalled()
    })

    it('успешно завершает диалог: добавляет summary, обновляет quota, ставит dialogCompleted=true', async () => {
      const setMessages = vi.fn()
      const setDialogCompleted = vi.fn()
      const setQuota = vi.fn()
      const setSummaryHistoryVisible = vi.fn()
      const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        expect(String(input)).toContain('/api/chat/complete')
        expect(JSON.parse(String(init?.body))).toEqual({ dialogId: 100, responseMode: 'live' })
        return jsonResponse({
          ok: true,
          summary: { id: 500, role: 'summary', content: 'Итог диалога', createdAt: '2026-01-01T00:00:00Z' },
          quota: { used: 5, limit: 5, remaining: 0 },
        })
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({
        setMessages, setDialogCompleted, setQuota, setSummaryHistoryVisible,
      }))
      await apiActions.completeDialog()

      expect(setSummaryHistoryVisible).toHaveBeenCalledWith(true)
      const result = setMessages.mock.calls[0][0]([{ id: 1, role: 'user', content: 'x' }])
      expect(result).toEqual([{ id: 1, role: 'user', content: 'x' }, { id: 500, role: 'summary', content: 'Итог диалога', createdAt: '2026-01-01T00:00:00Z' }])
      expect(setDialogCompleted).toHaveBeenCalledWith(true)
      expect(setQuota).toHaveBeenCalledWith({ used: 5, limit: 5, remaining: 0 })
    })

    it('при ошибке сервера пишет error и НЕ ставит dialogCompleted', async () => {
      const setError = vi.fn()
      const setDialogCompleted = vi.fn()
      const fetchMock = vi.fn(async () => jsonResponse({ ok: false, error: 'нельзя завершить' }))
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ setError, setDialogCompleted }))
      await apiActions.completeDialog()

      expect(setError).toHaveBeenCalledWith('нельзя завершить')
      expect(setDialogCompleted).not.toHaveBeenCalled()
    })
  })

  describe('loadHistory', () => {
    it('первичная загрузка (more=false): подставляет dialogId/modeId, скроллит к началу', async () => {
      const setDialogId = vi.fn()
      const setModeId = vi.fn()
      const setMessages = vi.fn()
      const setBeforeId = vi.fn()
      const scrollChatToBottom = vi.fn()
      const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
        expect(String(input)).toContain('dialogId=200')
        expect(String(input)).toContain('limit=20')
        return jsonResponse({
          ok: true,
          dialogId: 200,
          modeId: 30,
          modeName: 'Режим 30',
          messages: [{ id: 1, role: 'user', content: 'a', createdAt: '2026-01-01T00:00:00Z' }],
        })
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({
        setDialogId, setModeId, setMessages, setBeforeId, scrollChatToBottom,
      }))
      const result = await apiActions.loadHistory(200)

      expect(result).toBe(true)
      expect(setDialogId).toHaveBeenCalledWith(200)
      expect(setModeId).toHaveBeenCalledWith(30)
      expect(setBeforeId).toHaveBeenCalledWith(1)
      expect(scrollChatToBottom).toHaveBeenCalledWith('auto')
    })

    it('more=true: добавляет beforeId в query и НЕ скроллит/НЕ трогает dialogId', async () => {
      const setDialogId = vi.fn()
      const setBeforeId = vi.fn()
      const scrollChatToBottom = vi.fn()
      const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
        expect(String(input)).toContain('beforeId=50')
        return jsonResponse({ ok: true, messages: [{ id: 10, role: 'user', content: 'a', createdAt: '2026-01-01T00:00:00Z' }] })
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ beforeId: 50, setDialogId, setBeforeId, scrollChatToBottom }))
      await apiActions.loadHistory(200, true)

      expect(setDialogId).not.toHaveBeenCalled()
      expect(scrollChatToBottom).not.toHaveBeenCalled()
    })

    it('на 403 ставит dialogAccessRequired=true, чистит input, возвращает false', async () => {
      const setDialogAccessRequired = vi.fn()
      const setInput = vi.fn()
      const setError = vi.fn()
      const fetchMock = vi.fn(async () => jsonResponse({}, { status: 403 }))
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ setDialogAccessRequired, setInput, setError }))
      const result = await apiActions.loadHistory(200)

      expect(result).toBe(false)
      expect(setDialogAccessRequired).toHaveBeenCalledWith(true)
      expect(setInput).toHaveBeenCalledWith('')
      expect(setError).toHaveBeenCalledWith(expect.stringContaining('Продлите доступ'))
    })

    it('на 404 пишет специфичную ошибку "не найден", возвращает false', async () => {
      const setError = vi.fn()
      const fetchMock = vi.fn(async () => jsonResponse({}, { status: 404 }))
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ setError }))
      const result = await apiActions.loadHistory(200)

      expect(result).toBe(false)
      expect(setError).toHaveBeenCalledWith(expect.stringContaining('не найден'))
    })
  })

  describe('startNewDialogAfterSummary', () => {
    it('если уже идёт startingAfterSummary — ничего не делает', async () => {
      const fetchMock = vi.fn()
      globalThis.fetch = fetchMock as unknown as typeof fetch
      await createChatApiActions(createContext({ startingAfterSummary: true })).startNewDialogAfterSummary('текст')
      expect(fetchMock).not.toHaveBeenCalled()
    })

    it('с modeId вызывает openMode(modeId, preservedInput, true) — newDialog=true', async () => {
      const setStartingAfterSummary = vi.fn()
      const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        expect(JSON.parse(String(init?.body))).toEqual({ modeId: 10, newDialog: true })
        return jsonResponse({ ok: true, dialogId: 1, modeName: 'M', modeId: 10 })
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ modeId: 10, setStartingAfterSummary }))
      await apiActions.startNewDialogAfterSummary('сохранённый')

      expect(setStartingAfterSummary).toHaveBeenNthCalledWith(1, true)
      expect(setStartingAfterSummary).toHaveBeenNthCalledWith(2, false)
    })

    it('без modeId вызывает openBaseMode(preservedInput, true) — базовый режим, не select-mode с modeId', async () => {
      const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        expect(JSON.parse(String(init?.body))).toEqual({ modeId: 0, newDialog: true })
        return jsonResponse({ ok: true, dialogId: 1, quota: null })
      })
      globalThis.fetch = fetchMock as unknown as typeof fetch

      const apiActions = createChatApiActions(createContext({ modeId: '', setStartingAfterSummary: vi.fn() }))
      await apiActions.startNewDialogAfterSummary('текст')

      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
  })
})
