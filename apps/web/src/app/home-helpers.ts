import { OPERATOR, OPERATOR_TELEGRAM_URL } from '@/lib/operator'

export type DemoMode = {
  id: number
  name: string
  demoChat?: string
  demo_chat?: string
}

export type DemoMessage = {
  role: 'user' | 'assistant'
  content: string
}

export type HomeProfile = {
  user: { role: string }
  hasAccess?: boolean
}

export const fallbackModes: DemoMode[] = []
export const TEST_ACCESS_MESSAGE = 'Добрый день! Вышлите пожалуйста мне тестовый доступ к Стратуму!'

export function shuffle<T>(items: T[]) {
  const copy = [...items]
  for (let i = copy.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[copy[i], copy[j]] = [copy[j], copy[i]]
  }
  return copy
}

export function resolveDemoChat(mode: DemoMode): string {
  return String(mode.demoChat || mode.demo_chat || '').trim()
}

export function parseDemoChat(mode?: DemoMode): DemoMessage[] {
  const raw = mode ? resolveDemoChat(mode) : ''
  if (!raw) return []

  try {
    const parsed = JSON.parse(raw)
    const source = Array.isArray(parsed) ? parsed : Array.isArray(parsed?.messages) ? parsed.messages : []
    const messages = source
      .map((item: any) => ({
        role: item.role === 'assistant' ? 'assistant' : 'user',
        content: String(item.content || item.text || '').trim(),
      }))
      .filter((item: DemoMessage) => item.content)

    if (messages.length) return messages.slice(0, 4)
  } catch {
    // Plain text demo is parsed below.
  }

  const lines = raw
    .split(/\n+/)
    .map(line => line.trim())
    .filter(Boolean)

  const messages: DemoMessage[] = lines.map((line, index) => {
    const normalized = line.toLowerCase()
    const assistant = normalized.startsWith('assistant:') || normalized.startsWith('ассистент:') || normalized.startsWith('помощник')
    const user = normalized.startsWith('user:') || normalized.startsWith('пользователь:')
    const content = line
      .replace(/^user:\s*/i, '')
      .replace(/^пользователь:\s*/i, '')
      .replace(/^assistant:\s*/i, '')
      .replace(/^ассистент:\s*/i, '')
      .replace(/^помощник[^:]*:\s*/i, '')
      .trim()

    return {
      role: assistant ? 'assistant' : user ? 'user' : index % 2 === 0 ? 'user' : 'assistant',
      content,
    }
  })

  return messages.filter(item => item.content).slice(0, 4)
}

export function buildTelegramUrl(message: string): string {
  const text = message.trim()
  if (!text) return OPERATOR_TELEGRAM_URL

  return `${OPERATOR_TELEGRAM_URL}?text=${encodeURIComponent(text)}`
}

export function buildTelegramAppUrl(message: string): string | null {
  if (typeof window === 'undefined') return null

  const text = message.trim()
  if (!text) return `tg://resolve?domain=${OPERATOR.telegramHandle}`

  return `tg://resolve?domain=${OPERATOR.telegramHandle}&text=${encodeURIComponent(text)}`
}
