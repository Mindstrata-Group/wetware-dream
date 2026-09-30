export type CheckStatus = 'ok' | 'warn' | 'fail' | 'skip' | 'running'
export type CheckItem = { id: string; name: string; status: CheckStatus; detail?: string; json?: unknown }
export type CheckGroup = { id: string; label: string; checks: CheckItem[] }
export type TesterStats = { totalChecks: number; passed: number; warned: number; failed: number; lastRun?: string }
export type SystemStatus = { counts?: Record<string, unknown>; [key: string]: unknown }
