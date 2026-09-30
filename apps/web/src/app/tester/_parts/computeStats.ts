import type { CheckGroup, TesterStats } from './_types'

function computeStats(groups: CheckGroup[]): TesterStats {
  const all = groups.flatMap(g => g.checks)
  return {
    totalChecks: all.length,
    passed: all.filter(c => c.status === 'ok').length,
    warned: all.filter(c => c.status === 'warn').length,
    failed: all.filter(c => c.status === 'fail').length,
  }
}

export { computeStats }
