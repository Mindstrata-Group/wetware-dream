'use client'
import { Suspense, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { BrandLogo } from '@/components/BrandLogo'
import { apiFetch } from '@/lib/api'
import type {
  CheckStatus, CheckItem, CheckGroup, TesterStats, SystemStatus,
} from './_parts/_types'
import { StatusIcon } from './_parts/StatusIcon'
import { CheckRow } from './_parts/CheckRow'
import { HScrollTabs } from './_parts/HScrollTabs'
import { buildChecksFromStatus } from './_parts/buildChecksFromStatus'
import { computeStats } from './_parts/computeStats'
function TesterContent() {
  const [groups, setGroups] = useState<CheckGroup[]>([])
  const [activeGroup, setActiveGroup] = useState('')
  const [running, setRunning] = useState(false)
  const [lastRun, setLastRun] = useState<string>('')
  const [loadError, setLoadError] = useState('')
  const [mobile, setMobile] = useState(false)
  useEffect(() => {
    const check = () => setMobile(window.innerWidth < 768)
    check(); window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])
  const runChecks = useCallback(async () => {
    setRunning(true)
    setLoadError('')
    try {
      let newGroups: CheckGroup[] = []
      try {
        const resp = await apiFetch<{ result?: { results?: CheckItem[] }; groups?: CheckGroup[]; checks?: CheckItem[] }>('/api/tester/run-check', {
          method: 'POST',
          body: JSON.stringify({ check: 'all_safe' }),
        })
        if (resp.groups?.length) {
          newGroups = resp.groups
        } else if (resp.checks?.length) {
          newGroups = [{ id: 'all', label: 'Все проверки', checks: resp.checks }]
        } else if (resp.result?.results?.length) {
          newGroups = [{ id: 'all', label: 'Все проверки', checks: resp.result.results }]
        }
      } catch {
        const status = await apiFetch<SystemStatus>('/api/admin/status')
        newGroups = buildChecksFromStatus(status)
        newGroups.unshift({
          id: 'api',
          label: 'API',
          checks: [
            { id: 'api-health', name: 'Health endpoint', status: 'ok', detail: 'POST /api/tester/run-check или fallback status', json: { request: { method: 'POST', endpoint: '/api/tester/run-check (fallback: /api/admin/status)' }, response: { ok: true } } },
          ],
        })
      }
      try {
        const endpoint = '/api/public/demo-modes'
        const modesResp = await apiFetch<{ modes?: unknown[] }>(endpoint)
        const count = modesResp.modes?.length ?? 0
        const modeCheck: CheckItem = {
          id: 'api-modes',
          name: 'Количество режимов (system)',
          status: count > 0 ? 'ok' : 'warn',
          detail: count > 0 ? `Запрос: ${endpoint}; получено ${count} режимов` : `Запрос: ${endpoint}; режимов нет`,
          json: { endpoint, response: modesResp },
        }
        const apiGroup = newGroups.find(g => g.id === 'api')
        if (apiGroup) apiGroup.checks.push(modeCheck)
        else newGroups.push({ id: 'api', label: 'API', checks: [modeCheck] })
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'endpoint unavailable'
        const apiGroup = newGroups.find(g => g.id === 'api')
        const failCheck: CheckItem = { id: 'api-modes', name: 'Количество режимов (system)', status: 'fail', detail: 'GET /api/admin/status', json: { request: { method: 'GET', endpoint: '/api/admin/status' }, response: { error: msg } } }
        if (apiGroup) apiGroup.checks.push(failCheck)
        else newGroups.push({ id: 'api', label: 'API', checks: [failCheck] })
      }
      try {
        const endpoint = '/api/public/demo-modes'
        const demoResp = await apiFetch<{ modes?: unknown[] }>(endpoint)
        const count = demoResp.modes?.length ?? 0
        const demoCheck: CheckItem = {
          id: 'api-demo-modes',
          name: 'Публичные demo режимы',
          status: count > 0 ? 'ok' : 'warn',
          detail: `GET ${endpoint}`,
          json: { request: { method: 'GET', endpoint }, response: demoResp, extracted: { modes: count } },
        }
        const apiGroup = newGroups.find(g => g.id === 'api')
        if (apiGroup) apiGroup.checks.push(demoCheck)
        else newGroups.push({ id: 'api', label: 'API', checks: [demoCheck] })
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'endpoint unavailable'
        const apiGroup = newGroups.find(g => g.id === 'api')
        const failCheck: CheckItem = {
          id: 'api-demo-modes',
          name: 'Публичные demo режимы',
          status: 'fail',
          detail: 'GET /api/public/demo-modes',
          json: { request: { method: 'GET', endpoint: '/api/public/demo-modes' }, response: { error: msg } },
        }
        if (apiGroup) apiGroup.checks.push(failCheck)
        else newGroups.push({ id: 'api', label: 'API', checks: [failCheck] })
      }
      try {
        await apiFetch('/api/auth/me')
        const authCheck: CheckItem = { id: 'auth-me', name: 'Auth /me', status: 'ok', detail: 'GET /api/auth/me', json: { request: { method: 'GET', endpoint: '/api/auth/me' }, response: { ok: true } } }
        const apiGroup = newGroups.find(g => g.id === 'api')
        if (apiGroup) apiGroup.checks.push(authCheck)
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'Ошибка'
        const authCheck: CheckItem = { id: 'auth-me', name: 'Auth /me', status: msg === 'auth required' ? 'warn' : 'fail', detail: 'GET /api/auth/me', json: { request: { method: 'GET', endpoint: '/api/auth/me' }, response: { error: msg } } }
        const apiGroup = newGroups.find(g => g.id === 'api')
        if (apiGroup) apiGroup.checks.push(authCheck)
      }
      setGroups(newGroups)
      if (newGroups.length > 0 && !newGroups.find(g => g.id === activeGroup)) {
        setActiveGroup(newGroups[0].id)
      }
      setLastRun(new Date().toLocaleTimeString('ru-RU'))
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : 'Ошибка загрузки')
    } finally {
      setRunning(false)
    }
  }, [activeGroup])
  useEffect(() => { void runChecks() }, [])
  const stats = computeStats(groups)
  const passRate = stats.totalChecks > 0 ? Math.round((stats.passed / stats.totalChecks) * 100) : 0
  const currentGroup = groups.find(g => g.id === activeGroup) || groups[0]
  return (
    <div style={{ minHeight: '100vh', background: 'var(--background)', fontFamily: '"Golos Text", system-ui, sans-serif', color: 'var(--foreground)' }}>
      {/* Header */}
      <header style={{ position: 'sticky' as const, top: 0, zIndex: 20, height: 64, display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 32px', background: 'color-mix(in oklab, var(--card) 88%, transparent)', backdropFilter: 'blur(12px)', borderBottom: '1px solid var(--line)' }}>
        <BrandLogo priority />
        <div style={{ display: 'flex', gap: 8 }}>
          <Link href="/" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'color-mix(in oklab, var(--card) 88%, transparent)', color: 'var(--foreground)', fontSize: 13, fontWeight: 500, textDecoration: 'none' }}>← Главная</Link>
          <Link href="/profile" style={{ display: 'inline-flex', alignItems: 'center', height: 36, padding: '0 14px', borderRadius: 10, border: '1px solid var(--line)', background: 'color-mix(in oklab, var(--card) 88%, transparent)', color: 'var(--foreground)', fontSize: 13, textDecoration: 'none' }}>Профиль</Link>
          <button onClick={runChecks} disabled={running} style={{ height: 36, padding: '0 16px', borderRadius: 10, background: running ? 'var(--muted)' : '#1D9E75', color: '#fff', fontSize: 13, fontWeight: 500, border: 'none', cursor: running ? 'not-allowed' : 'pointer', fontFamily: 'inherit' }}>
            {running ? 'Запуск…' : '▶ Запустить все'}
          </button>
        </div>
      </header>
      <div style={{ maxWidth: 1100, margin: '0 auto', padding: '40px 32px' }}>
        {/* Title */}
        <div style={{ marginBottom: 28 }}>
          <h1 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontWeight: 600, fontSize: mobile ? 20 : 28, letterSpacing: '-0.02em', margin: '0 0 6px' }}>Тестер</h1>
          <p style={{ fontSize: 14, color: 'var(--muted)', margin: 0 }}>
            Системные проверки{stats.totalChecks > 0 ? ` · Успешность: ${passRate}%` : ''}{lastRun ? ` · Последний запуск: ${lastRun}` : ''}
          </p>
        </div>
        {loadError && (
          <div style={{ padding: '12px 16px', borderRadius: 10, background: '#FBF0F0', border: '1px solid #EFB9B9', color: '#8a3333', fontSize: 13, marginBottom: 20 }}>
            {loadError}
            <button onClick={runChecks} style={{ marginLeft: 12, fontSize: 12, color: '#C2453E', background: 'none', border: 'none', cursor: 'pointer', fontFamily: 'inherit', textDecoration: 'underline' }}>Повторить</button>
          </div>
        )}
        {/* Stats */}
        {stats.totalChecks > 0 && (
          <div style={{ display: 'grid', gridTemplateColumns: mobile ? 'repeat(2,1fr)' : 'repeat(4,1fr)', gap: 12, marginBottom: 24 }}>
            {[
              { label: 'Всего проверок', value: stats.totalChecks, color: 'var(--foreground)' },
              { label: 'Прошли', value: stats.passed, color: '#1D9E75' },
              { label: 'Предупреждения', value: stats.warned, color: '#B07A1A' },
              { label: 'Ошибки', value: stats.failed, color: '#C2453E' },
            ].map(item => (
              <div key={item.label} style={{ background: 'var(--card)', border: '1px solid var(--line)', borderRadius: 12, padding: '16px 18px' }}>
                <div style={{ fontSize: mobile ? 22 : 28, fontWeight: 700, fontFamily: '"Unbounded", system-ui', color: item.color, lineHeight: 1 }}>{item.value}</div>
                <div style={{ fontSize: 12, color: 'var(--muted)', marginTop: 6 }}>{item.label}</div>
              </div>
            ))}
          </div>
        )}
        {running && groups.length === 0 && (
          <div style={{ textAlign: 'center', padding: '60px 20px', color: 'var(--muted)', fontSize: 14 }}>Запускаем проверки…</div>
        )}
        {groups.length > 0 && currentGroup && (
          <div style={{ display: 'flex', gap: 24, alignItems: 'flex-start' }}>
            {/* Sidebar (desktop) */}
            {!mobile && (
              <nav style={{ width: 180, flexShrink: 0, position: 'sticky' as const, top: 88, alignSelf: 'flex-start' }}>
                {groups.map(g => {
                  const errs = g.checks.filter(c => c.status === 'fail').length
                  const warns = g.checks.filter(c => c.status === 'warn').length
                  return (
                    <button key={g.id} onClick={() => setActiveGroup(g.id)} style={{
                      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                      width: '100%', padding: '9px 12px', borderRadius: 8,
                      background: g.id === activeGroup ? '#E1F5EE' : 'transparent',
                      border: 'none', cursor: 'pointer', fontFamily: 'inherit', marginBottom: 2,
                    }}>
                      <span style={{ fontSize: 14, color: g.id === activeGroup ? '#0F6E56' : 'var(--foreground)', fontWeight: g.id === activeGroup ? 500 : 400 }}>{g.label}</span>
                      {errs > 0 && <span style={{ fontSize: 11, fontWeight: 700, color: '#C2453E' }}>{errs}</span>}
                      {errs === 0 && warns > 0 && <span style={{ fontSize: 11, fontWeight: 700, color: '#B07A1A' }}>{warns}</span>}
                    </button>
                  )
                })}
              </nav>
            )}
            {/* Checks panel */}
            <div style={{ flex: 1, minWidth: 0 }}>
              {mobile && (
                <div style={{ marginBottom: 16 }}>
                  <HScrollTabs groups={groups} active={activeGroup} onSelect={setActiveGroup} />
                </div>
              )}
              <div style={{ background: 'var(--card)', border: '1px solid var(--line)', borderRadius: 16, padding: '20px 24px' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
                  <h2 style={{ fontFamily: '"Unbounded",system-ui', fontWeight: 600, fontSize: 16, margin: 0 }}>{currentGroup.label}</h2>
                  <button onClick={runChecks} disabled={running} style={{ fontSize: 12, color: '#0F6E56', background: 'none', border: 'none', cursor: running ? 'not-allowed' : 'pointer', fontFamily: 'inherit', fontWeight: 500 }}>
                    {running ? 'Запуск…' : 'Перепроверить'}
                  </button>
                </div>
                {currentGroup.checks.length === 0 ? (
                  <div style={{ color: 'var(--muted)', fontSize: 14, padding: '10px 0' }}>Нет данных</div>
                ) : (
                  currentGroup.checks.map(c => <CheckRow key={c.id} check={c} />)
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
export default function TesterPage() {
  return (
    <Suspense fallback={<div style={{ minHeight: '100vh', background: 'var(--background)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--muted)', fontFamily: '"Golos Text",system-ui,sans-serif' }}>Загрузка…</div>}>
      <TesterContent />
    </Suspense>
  )
}
