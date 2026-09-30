import { describe, expect, it } from 'vitest'
import { parseDemoChat } from './utils'

const mode = (demoChat?: string, demo_chat?: string) => ({ id: 1, name: 'Test', demoChat, demo_chat })

describe('parseDemoChat (_home/utils)', () => {
  it('returns empty array for empty/missing content', () => {
    // kills the if (!raw) -> if (raw) mutation
    expect(parseDemoChat(mode('', ''))).toEqual([])
    expect(parseDemoChat(mode(undefined, undefined))).toEqual([])
  })

  it('prefers demoChat over demo_chat (убивает || порядок мутацию)', () => {
    const result = parseDemoChat(mode(
      JSON.stringify([{ role: 'user', content: 'из demoChat' }]),
      JSON.stringify([{ role: 'user', content: 'из demo_chat' }]),
    ))
    expect(result[0].content).toBe('из demoChat')
  })

  it('falls back to demo_chat when demoChat absent (убивает || мутацию)', () => {
    const result = parseDemoChat(mode(
      undefined,
      JSON.stringify([{ role: 'user', content: 'из demo_chat' }]),
    ))
    expect(result[0].content).toBe('из demo_chat')
  })

  it('parses JSON array of messages', () => {
    // kills the Array.isArray(parsed) -> false mutation
    const msgs = [
      { role: 'user', content: 'Привет' },
      { role: 'assistant', content: 'Здравствуйте' },
    ]
    const result = parseDemoChat(mode(JSON.stringify(msgs)))
    expect(result).toEqual(msgs)
  })

  it('parses JSON object with messages array (убивает Array.isArray(parsed?.messages) мутацию)', () => {
    const msgs = { messages: [{ role: 'user', content: 'Вопрос' }, { role: 'assistant', content: 'Ответ' }] }
    const result = parseDemoChat(mode(JSON.stringify(msgs)))
    expect(result).toHaveLength(2)
    expect(result[0].content).toBe('Вопрос')
  })

  it('normalizes role: unknown roles become user (убивает === "assistant" мутацию)', () => {
    // kills: item.role === "assistant" -> item.role !== "assistant"
    const msgs = [
      { role: 'assistant', content: 'Ответ' },
      { role: 'unknown', content: 'Что-то' },
    ]
    const result = parseDemoChat(mode(JSON.stringify(msgs)))
    expect(result[0].role).toBe('assistant')
    expect(result[1].role).toBe('user')
  })

  it('falls back to text field when content missing (убивает || "text" мутацию)', () => {
    const msgs = [{ role: 'user', text: 'через text поле' }]
    const result = parseDemoChat(mode(JSON.stringify(msgs)))
    expect(result[0].content).toBe('через text поле')
  })

  it('filters out messages with empty content (убивает filter(m.content) мутацию)', () => {
    const msgs = [
      { role: 'user', content: '' },
      { role: 'assistant', content: 'Непустой' },
    ]
    const result = parseDemoChat(mode(JSON.stringify(msgs)))
    expect(result).toHaveLength(1)
    expect(result[0].content).toBe('Непустой')
  })

  it('returns empty array for non-JSON string', () => {
    // kills: try-catch catch -> removed (throws and fails)
    expect(parseDemoChat(mode('не JSON вообще'))).toEqual([])
  })

  it('returns empty array for JSON without array structure (убивает || [] fallback)', () => {
    expect(parseDemoChat(mode(JSON.stringify({ notMessages: 'x' })))).toEqual([])
  })
})
