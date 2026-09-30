import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { installPointerHandlers } from './interaction'
import { makeDot, pickDotKind } from './physics'
import { renderConnections, renderDots, renderPointerFeedback } from './render'
import type { Dot, PointerState } from './types'

function dot(partial: Partial<Dot>): Dot {
  return {
    x: 0,
    y: 0,
    vx: 0,
    vy: 0,
    r: 1,
    a: 1,
    kind: 'neutral',
    charge: 0,
    baseKind: 'neutral',
    baseCharge: 0,
    antiUntil: 0,
    breakCooldownUntil: 0,
    visible: true,
    respawnAt: 0,
    ...partial,
  }
}

function ctxMock() {
  return {
    strokeStyle: '',
    fillStyle: '',
    lineWidth: 0,
    globalAlpha: 1,
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
  } as unknown as CanvasRenderingContext2D
}

// ctxMock does not track the globalAlpha history (it is overwritten on every
// loop iteration); to check the alpha formulas precisely on every draw call
// we need a write tracker via an accessor property.
function ctxMockWithAlphaHistory() {
  const alphaHistory: number[] = []
  const ctx = {
    strokeStyle: '',
    fillStyle: '',
    lineWidth: 0,
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
  }
  Object.defineProperty(ctx, 'globalAlpha', {
    get: () => alphaHistory[alphaHistory.length - 1] ?? 1,
    set: (v: number) => { alphaHistory.push(v) },
  })
  return { ctx: ctx as unknown as CanvasRenderingContext2D, alphaHistory }
}

describe('home constellation physics helpers', () => {
  beforeEach(() => vi.restoreAllMocks())
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it.each([
    [0.01, 'attractor'],
    [0.14, 'attractor'],  // kills the < 0.15 -> <= 0.15 mutation
    [0.15, 'repeller'],   // kills the < 0.15 -> <= 0.14 mutation
    [0.20, 'repeller'],
    [0.29, 'repeller'],   // kills the < 0.30 -> <= 0.30 mutation
    [0.30, 'neutral'],    // kills the < 0.30 -> <= 0.29 mutation
    [0.90, 'neutral'],
  ] as const)('maps random roll %s to %s dot kind', (roll, kind) => {
    vi.spyOn(Math, 'random').mockReturnValue(roll)
    expect(pickDotKind()).toBe(kind)
  })

  it('creates dots within canvas bounds and keeps charge/base kind invariant', () => {
    vi.spyOn(Math, 'random')
      .mockReturnValueOnce(0.1) // attractor
      .mockReturnValueOnce(0.25) // x
      .mockReturnValueOnce(0.5) // y
      .mockReturnValueOnce(0.75)
      .mockReturnValueOnce(0.25)
      .mockReturnValueOnce(0.5)
      .mockReturnValueOnce(1)

    expect(makeDot(200, 100)).toMatchObject({
      x: 50,
      y: 50,
      kind: 'attractor',
      charge: 1,
      baseKind: 'attractor',
      baseCharge: 1,
      visible: true,
      antiUntil: 0,
    })
  })

  it('makeDot: точные значения ВСЕХ вычисляемых числовых полей (vx/vy/r/a) — убивает арифметические мутации формул', () => {
    vi.spyOn(Math, 'random')
      .mockReturnValueOnce(0.9) // neutral (>= 0.30)
      .mockReturnValueOnce(0.25) // x roll
      .mockReturnValueOnce(0.5) // y roll
      .mockReturnValueOnce(0.75) // vx roll
      .mockReturnValueOnce(0.25) // vy roll
      .mockReturnValueOnce(0.5) // r roll
      .mockReturnValueOnce(1) // a roll

    expect(makeDot(200, 100)).toEqual({
      x: 0.25 * 200,
      y: 0.5 * 100,
      vx: (0.75 - 0.5) * 0.31,
      vy: (0.25 - 0.5) * 0.31,
      r: 0.5 * 1.6 + 0.5,
      a: 0.35 + 1 * 0.55,
      kind: 'neutral',
      charge: 0,
      baseKind: 'neutral',
      baseCharge: 0,
      antiUntil: 0,
      breakCooldownUntil: 0,
      visible: true,
      respawnAt: 0,
    })
  })

  it.each([
    [0.05, 'attractor', 1],
    [0.20, 'repeller', -1],
    [0.90, 'neutral', 0],
  ] as const)('makeDot: kind=%s → charge=%d (убивает тернарную цепочку kind===attractor?1:kind===repeller?-1:0)', (roll, kind, charge) => {
    vi.spyOn(Math, 'random').mockReturnValue(roll)
    const result = makeDot(100, 100)
    expect(result.kind).toBe(kind)
    expect(result.charge).toBe(charge)
    expect(result.baseKind).toBe(kind)
    expect(result.baseCharge).toBe(charge)
  })
})

describe('home constellation render helpers', () => {
  it('renders only visible nearby connections and returns neighbor counts', () => {
    const ctx = ctxMock()
    const counts = renderConnections(ctx, [dot({ x: 0, y: 0 }), dot({ x: 10, y: 0, antiUntil: 200 }), dot({ x: 500, y: 0 }), dot({ x: 0, y: 0, visible: false })], 100, true)

    expect(counts).toEqual([1, 1, 0, 0])
    expect(ctx.beginPath).toHaveBeenCalledTimes(1)
    expect(ctx.stroke).toHaveBeenCalledTimes(1)
  })

  it('renders visible dots and restores global alpha', () => {
    const ctx = ctxMock()
    renderDots(ctx, [dot({ a: 0.5 }), dot({ visible: false })])

    expect(ctx.arc).toHaveBeenCalledTimes(1)
    expect(ctx.fill).toHaveBeenCalledTimes(1)
    expect(ctx.globalAlpha).toBe(1)
  })

  it('renders hold and burst pointer feedback', () => {
    const ctx = ctxMock()
    const pointer: PointerState = { x: 5, y: 6, hovering: true, holding: true, holdT0: 0, burst: { x: 7, y: 8, t0: 0, strength: 300 } }

    renderPointerFeedback(ctx, pointer, 1000, 0.1, true)

    expect(ctx.arc).toHaveBeenCalledTimes(3)
    expect(ctx.stroke).toHaveBeenCalledTimes(2)
    expect(ctx.fill).toHaveBeenCalledTimes(1)
    expect(ctx.globalAlpha).toBe(1)
  })

  it('стиль/ширина линии соединений — точные значения (убивает мутации строк/чисел)', () => {
    const ctx = ctxMock()
    renderConnections(ctx, [dot({ x: 0, y: 0 }), dot({ x: 10, y: 0 })], 0, false)
    expect(ctx.strokeStyle).toBe('#1D9E75')
    expect(ctx.lineWidth).toBe(1)
  })

  it('moveTo/lineTo вызываются с точными координатами точек пары', () => {
    const ctx = ctxMock()
    renderConnections(ctx, [dot({ x: 3, y: 4 }), dot({ x: 13, y: 4 })], 0, false)
    expect(ctx.moveTo).toHaveBeenCalledWith(3, 4)
    expect(ctx.lineTo).toHaveBeenCalledWith(13, 4)
  })

  it('dist < lineReach(92) — граница включения соединения (убивает < → <= мутацию)', () => {
    // Exactly 92 must NOT connect (strict <).
    const atBoundary = ctxMockWithAlphaHistory()
    renderConnections(atBoundary.ctx, [dot({ x: 0, y: 0 }), dot({ x: 92, y: 0 })], 0, false)
    expect(atBoundary.ctx.beginPath).not.toHaveBeenCalled()

    // 91.99 is just below the threshold and must connect.
    const justInside = ctxMockWithAlphaHistory()
    renderConnections(justInside.ctx, [dot({ x: 0, y: 0 }), dot({ x: 91.99, y: 0 })], 0, false)
    expect(justInside.ctx.beginPath).toHaveBeenCalledTimes(1)
  })

  it('globalAlpha формула (1 - dist/92) * 0.14 без antiFade (interactionsEnabled=false)', () => {
    const { ctx, alphaHistory } = ctxMockWithAlphaHistory()
    // dist=10 → (1 - 10/92) * 0.14 = 0.8913043478260869 * 0.14
    renderConnections(ctx, [dot({ x: 0, y: 0 }), dot({ x: 10, y: 0, antiUntil: 99999 })], 0, false)
    const expected = (1 - 10 / 92) * 0.14 * 1
    expect(alphaHistory[alphaHistory.length - 1]).toBeCloseTo(expected, 10)
  })

  it('antiFade=0.65 применяется только когда interactionsEnabled=true И antiUntil>now у одной из точек', () => {
    const dist = 10
    const base = (1 - dist / 92) * 0.14

    // interactionsEnabled=true, but antiUntil is in the past -> antiFade=1 (not 0.65).
    const noAnti = ctxMockWithAlphaHistory()
    renderConnections(noAnti.ctx, [dot({ x: 0, y: 0, antiUntil: 0 }), dot({ x: dist, y: 0, antiUntil: 0 })], 100, true)
    expect(noAnti.alphaHistory[noAnti.alphaHistory.length - 1]).toBeCloseTo(base, 10)

    // interactionsEnabled=true AND antiUntil>now for the second point -> antiFade=0.65.
    const withAnti = ctxMockWithAlphaHistory()
    renderConnections(withAnti.ctx, [dot({ x: 0, y: 0, antiUntil: 0 }), dot({ x: dist, y: 0, antiUntil: 200 })], 100, true)
    expect(withAnti.alphaHistory[withAnti.alphaHistory.length - 1]).toBeCloseTo(base * 0.65, 10)

    // interactionsEnabled=false: antiFade is ignored even if antiUntil>now.
    const disabledAnti = ctxMockWithAlphaHistory()
    renderConnections(disabledAnti.ctx, [dot({ x: 0, y: 0, antiUntil: 0 }), dot({ x: dist, y: 0, antiUntil: 200 })], 100, false)
    expect(disabledAnti.alphaHistory[disabledAnti.alphaHistory.length - 1]).toBeCloseTo(base, 10)
  })

  it('maxLinksPerDot=5: точка с 6+ связями пропускает отрисовку только СВОИХ избыточных пар (убивает > → >= и || → && мутации)', () => {
    // 7 points on one straight line (step 5, max distance 30 << 92):
    // ALL 21 pairs are geometrically close, so the only reason
    // a line is skipped is maxLinksPerDot, not the dist threshold.
    // Hand-traced sequence: each point takes part in 6
    // pairs; exactly 6 pairs are skipped (one per point, when its
    // counter becomes the 6th), 21-6=15 draws, final counts=[6]*7.
    const ctx = ctxMock()
    const dots = Array.from({ length: 7 }, (_, k) => dot({ x: 5 * k, y: 0 }))
    const counts = renderConnections(ctx, dots, 0, false)

    expect(counts).toEqual([6, 6, 6, 6, 6, 6, 6])
    expect(ctx.beginPath).toHaveBeenCalledTimes(15)
  })

  it('renderDots: fillStyle точный, globalAlpha = d.a * 0.55 на каждую точку, arc с точными параметрами', () => {
    const { ctx, alphaHistory } = ctxMockWithAlphaHistory()
    renderDots(ctx, [dot({ x: 1, y: 2, r: 3, a: 0.5 }), dot({ x: 4, y: 5, r: 6, a: 1 })])

    expect(ctx.fillStyle).toBe('#1D9E75')
    expect(alphaHistory).toEqual([0.5 * 0.55, 1 * 0.55, 1]) // 2 points + the final reset to 1
    expect(ctx.arc).toHaveBeenNthCalledWith(1, 1, 2, 3, 0, Math.PI * 2)
    expect(ctx.arc).toHaveBeenNthCalledWith(2, 4, 5, 6, 0, Math.PI * 2)
  })

  it('renderPointerFeedback: p.holding=false — ничего не рисует (убивает удаление if-guard мутацию)', () => {
    const ctx = ctxMock()
    const pointer: PointerState = { x: 0, y: 0, hovering: false, holding: false, holdT0: 0, burst: null }
    renderPointerFeedback(ctx, pointer, 1000, 0.1, false)
    expect(ctx.arc).not.toHaveBeenCalled()
    expect(ctx.stroke).not.toHaveBeenCalled()
    expect(ctx.fill).not.toHaveBeenCalled()
  })

  it('renderPointerFeedback: burstActive=false — burst-круг не рисуется, даже если p.burst задан', () => {
    const ctx = ctxMock()
    const pointer: PointerState = { x: 0, y: 0, hovering: false, holding: false, holdT0: 0, burst: { x: 1, y: 1, t0: 0, strength: 1 } }
    renderPointerFeedback(ctx, pointer, 1000, 0.1, false)
    expect(ctx.arc).not.toHaveBeenCalled()
  })

  it('renderPointerFeedback: точные формулы charge/ringR/alpha при holding=true (убивает арифметические мутации)', () => {
    const { ctx, alphaHistory } = ctxMockWithAlphaHistory()
    const pointer: PointerState = { x: 5, y: 6, hovering: true, holding: true, holdT0: 1000, burst: null }
    const now = 4000 // heldSec = 3
    renderPointerFeedback(ctx, pointer, now, 0, false)

    const heldSec = 3
    const charge = 1 + Math.log(1 + heldSec) * 1.5
    const ringR = Math.min(180, 14 + charge * 22)
    const ringAlpha = Math.min(0.55, 0.18 + charge * 0.05)
    const pulse = 1 + Math.sin(now / 110) * 0.25
    const dotAlpha = Math.min(0.8, 0.32 + charge * 0.06)
    const dotR = 2.5 * pulse + charge * 0.6

    expect(ctx.lineWidth).toBe(1.5)
    expect(ctx.arc).toHaveBeenNthCalledWith(1, 5, 6, ringR, 0, Math.PI * 2)
    expect(ctx.arc).toHaveBeenNthCalledWith(2, 5, 6, dotR, 0, Math.PI * 2)
    expect(alphaHistory[0]).toBeCloseTo(ringAlpha, 10)
    expect(alphaHistory[1]).toBeCloseTo(dotAlpha, 10)
    expect(alphaHistory[alphaHistory.length - 1]).toBe(1)
  })

  it('renderPointerFeedback: burst-круг — точный радиус r=burstAge*700 и альфа (1-burstAge/0.4)*0.5', () => {
    const { ctx, alphaHistory } = ctxMockWithAlphaHistory()
    const pointer: PointerState = { x: 0, y: 0, hovering: false, holding: false, holdT0: 0, burst: { x: 7, y: 8, t0: 0, strength: 300 } }
    const burstAge = 0.2
    renderPointerFeedback(ctx, pointer, 1000, burstAge, true)

    expect(ctx.lineWidth).toBe(2)
    expect(ctx.arc).toHaveBeenCalledWith(7, 8, burstAge * 700, 0, Math.PI * 2)
    const expectedAlpha = (1 - burstAge / 0.4) * 0.5
    expect(alphaHistory[0]).toBeCloseTo(expectedAlpha, 10)
    expect(alphaHistory[alphaHistory.length - 1]).toBe(1)
  })
})

describe('home constellation pointer interactions', () => {
  it('updates pointer state from window events and removes handlers on cleanup', () => {
    vi.spyOn(performance, 'now')
      .mockReturnValueOnce(1000)
      .mockReturnValueOnce(2500)
      .mockReturnValueOnce(2500)
    const canvas = document.createElement('canvas')
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 100, right: 110, bottom: 120, x: 10, y: 20, toJSON: () => ({}) })
    const ptr = { current: { x: 0, y: 0, hovering: false, holding: false, holdT0: 0, burst: null } satisfies PointerState }
    const cleanup = installPointerHandlers(canvas, ptr)

    const PointerCtor = window.PointerEvent ?? window.MouseEvent
    window.dispatchEvent(new PointerCtor('pointerdown', { clientX: 30, clientY: 50 }))
    expect(ptr.current).toMatchObject({ x: 20, y: 30, hovering: true, holding: true, holdT0: 1000, burst: null })

    window.dispatchEvent(new PointerCtor('pointerup'))
    expect(ptr.current.holding).toBe(false)
    expect(ptr.current.burst?.x).toBe(20)
    expect(ptr.current.burst?.strength).toBeGreaterThan(260)

    window.dispatchEvent(new PointerCtor('pointermove', { clientX: 40, clientY: 55 }))
    expect(ptr.current).toMatchObject({ x: 30, y: 35, hovering: true })

    window.dispatchEvent(new Event('blur'))
    expect(ptr.current.hovering).toBe(false)

    cleanup()
    window.dispatchEvent(new PointerCtor('pointermove', { clientX: 99, clientY: 99 }))
    expect(ptr.current.x).toBe(30)
  })

  it('onUp без holding — ранний return, burst не создаётся (убивает удаление if(!p.holding)return мутацию)', () => {
    const canvas = document.createElement('canvas')
    canvas.getBoundingClientRect = () => ({ left: 0, top: 0, width: 100, height: 100, right: 100, bottom: 100, x: 0, y: 0, toJSON: () => ({}) })
    const ptr = { current: { x: 5, y: 5, hovering: false, holding: false, holdT0: 0, burst: null } satisfies PointerState }
    const cleanup = installPointerHandlers(canvas, ptr)

    const PointerCtor = window.PointerEvent ?? window.MouseEvent
    window.dispatchEvent(new PointerCtor('pointerup'))

    expect(ptr.current.burst).toBeNull()
    expect(ptr.current.holding).toBe(false)
    cleanup()
  })

  it('strength = 260 + log(1 + heldSec*5)*460 — точная формула (убивает арифметические мутации)', () => {
    vi.spyOn(performance, 'now')
      .mockReturnValueOnce(0) // pointerdown holdT0
      .mockReturnValueOnce(3000) // pointerup: heldSec = 3
      .mockReturnValueOnce(3000) // burst.t0
    const canvas = document.createElement('canvas')
    canvas.getBoundingClientRect = () => ({ left: 0, top: 0, width: 100, height: 100, right: 100, bottom: 100, x: 0, y: 0, toJSON: () => ({}) })
    const ptr = { current: { x: 0, y: 0, hovering: false, holding: false, holdT0: 0, burst: null } satisfies PointerState }
    const cleanup = installPointerHandlers(canvas, ptr)

    const PointerCtor = window.PointerEvent ?? window.MouseEvent
    window.dispatchEvent(new PointerCtor('pointerdown', { clientX: 0, clientY: 0 }))
    window.dispatchEvent(new PointerCtor('pointerup'))

    const heldSec = 3
    const expectedStrength = 260 + Math.log(1 + heldSec * 5) * 460
    expect(ptr.current.burst?.strength).toBeCloseTo(expectedStrength, 10)
    cleanup()
  })

  it('pointercancel запускает тот же обработчик, что и pointerup (создаёт burst)', () => {
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(1000).mockReturnValueOnce(1000)
    const canvas = document.createElement('canvas')
    canvas.getBoundingClientRect = () => ({ left: 0, top: 0, width: 100, height: 100, right: 100, bottom: 100, x: 0, y: 0, toJSON: () => ({}) })
    const ptr = { current: { x: 0, y: 0, hovering: false, holding: false, holdT0: 0, burst: null } satisfies PointerState }
    const cleanup = installPointerHandlers(canvas, ptr)

    const PointerCtor = window.PointerEvent ?? window.MouseEvent
    window.dispatchEvent(new PointerCtor('pointerdown', { clientX: 0, clientY: 0 }))
    window.dispatchEvent(new PointerCtor('pointercancel'))

    expect(ptr.current.holding).toBe(false)
    expect(ptr.current.burst).not.toBeNull()
    cleanup()
  })

  it('mouseleave на document сбрасывает hovering в false', () => {
    const canvas = document.createElement('canvas')
    canvas.getBoundingClientRect = () => ({ left: 0, top: 0, width: 100, height: 100, right: 100, bottom: 100, x: 0, y: 0, toJSON: () => ({}) })
    const ptr = { current: { x: 0, y: 0, hovering: true, holding: false, holdT0: 0, burst: null } satisfies PointerState }
    const cleanup = installPointerHandlers(canvas, ptr)

    document.dispatchEvent(new Event('mouseleave'))
    expect(ptr.current.hovering).toBe(false)
    cleanup()
  })

  it('contextmenu: preventDefault вызывается только когда holding=true (убивает удаление if-guard мутацию)', () => {
    const canvas = document.createElement('canvas')
    canvas.getBoundingClientRect = () => ({ left: 0, top: 0, width: 100, height: 100, right: 100, bottom: 100, x: 0, y: 0, toJSON: () => ({}) })
    const ptr = { current: { x: 0, y: 0, hovering: false, holding: false, holdT0: 0, burst: null } satisfies PointerState }
    const cleanup = installPointerHandlers(canvas, ptr)

    const notHoldingEvent = new MouseEvent('contextmenu', { cancelable: true })
    window.dispatchEvent(notHoldingEvent)
    expect(notHoldingEvent.defaultPrevented).toBe(false)

    ptr.current.holding = true
    const holdingEvent = new MouseEvent('contextmenu', { cancelable: true })
    window.dispatchEvent(holdingEvent)
    expect(holdingEvent.defaultPrevented).toBe(true)

    cleanup()
  })

  it('cleanup() снимает ВСЕ обработчики, включая pointerdown/pointerup/contextmenu/mouseleave', () => {
    const canvas = document.createElement('canvas')
    canvas.getBoundingClientRect = () => ({ left: 0, top: 0, width: 100, height: 100, right: 100, bottom: 100, x: 0, y: 0, toJSON: () => ({}) })
    const ptr = { current: { x: 0, y: 0, hovering: false, holding: false, holdT0: 0, burst: null } satisfies PointerState }
    const cleanup = installPointerHandlers(canvas, ptr)
    cleanup()

    const PointerCtor = window.PointerEvent ?? window.MouseEvent
    window.dispatchEvent(new PointerCtor('pointerdown', { clientX: 5, clientY: 5 }))
    expect(ptr.current.holding).toBe(false) // handler removed: pointerdown had no effect

    ptr.current.hovering = true
    document.dispatchEvent(new Event('mouseleave'))
    // the mouseleave handler is removed: hovering must not be reset.
    expect(ptr.current.hovering).toBe(true)
  })
})
