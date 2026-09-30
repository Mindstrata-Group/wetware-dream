import type { CheckGroup, CheckItem, CheckStatus, SystemStatus } from './_types'

function buildChecksFromStatus(status: SystemStatus | null): CheckGroup[] {
  if (!status) return []

  const counts = status.counts || {}
  const systemChecks: CheckItem[] = Object.entries(counts).map(([key, value]) => ({
    id: `system-${key}`,
    name: key,
    status: 'ok' as CheckStatus,
    detail: String(value),
    json: {
      request: { method: 'GET', endpoint: '/api/admin/status' },
      response: { [key]: value },
    },
  }))

  const groups: CheckGroup[] = [
    {
      id: 'system',
      label: 'Система',
      checks: systemChecks.length > 0 ? systemChecks : [
        { id: 'system-ok', name: 'Сервис работает', status: 'ok', detail: 'API доступен' },
      ],
    },
  ]

  return groups
}

export { buildChecksFromStatus }
