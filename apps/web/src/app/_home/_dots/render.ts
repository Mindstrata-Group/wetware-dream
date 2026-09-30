import type { Dot, PointerState } from './types'
import { HP } from './physics'

export function renderConnections(ctx: CanvasRenderingContext2D, dots: Dot[], now: number, interactionsEnabled: boolean) {
  const neighborCounts = new Array(dots.length).fill(0)
  ctx.strokeStyle = '#1D9E75'
  ctx.lineWidth = 1

  for (let i = 0; i < dots.length; i++) {
    const a = dots[i]
    if (!a.visible) continue

    for (let j = i + 1; j < dots.length; j++) {
      const b = dots[j]
      if (!b.visible) continue
      const dist = Math.hypot(a.x - b.x, a.y - b.y)

      if (dist < HP.lineReach) {
        neighborCounts[i]++
        neighborCounts[j]++
        if (neighborCounts[i] > HP.maxLinksPerDot || neighborCounts[j] > HP.maxLinksPerDot) continue
        const antiFade = (interactionsEnabled && (a.antiUntil > now || b.antiUntil > now)) ? HP.antiLineAlpha : 1
        ctx.globalAlpha = (1 - dist / HP.lineReach) * 0.14 * antiFade
        ctx.beginPath()
        ctx.moveTo(a.x, a.y)
        ctx.lineTo(b.x, b.y)
        ctx.stroke()
      }
    }
  }

  return neighborCounts
}

export function renderDots(ctx: CanvasRenderingContext2D, dots: Dot[]) {
  ctx.fillStyle = '#1D9E75'
  for (const d of dots) {
    if (!d.visible) continue
    ctx.globalAlpha = d.a * 0.55
    ctx.beginPath()
    ctx.arc(d.x, d.y, d.r, 0, Math.PI * 2)
    ctx.fill()
  }
  ctx.globalAlpha = 1
}

export function renderPointerFeedback(
  ctx: CanvasRenderingContext2D,
  p: PointerState,
  now: number,
  burstAge: number,
  burstActive: boolean,
) {
  if (p.holding) {
    const heldSec = (now - p.holdT0) / 1000
    const charge = 1 + Math.log(1 + heldSec) * 1.5
    const ringR = Math.min(180, 14 + charge * 22)
    ctx.strokeStyle = '#1D9E75'
    ctx.lineWidth = 1.5
    ctx.globalAlpha = Math.min(0.55, 0.18 + charge * 0.05)
    ctx.beginPath()
    ctx.arc(p.x, p.y, ringR, 0, Math.PI * 2)
    ctx.stroke()

    const pulse = 1 + Math.sin(now / 110) * 0.25
    ctx.fillStyle = '#1D9E75'
    ctx.globalAlpha = Math.min(0.8, 0.32 + charge * 0.06)
    ctx.beginPath()
    ctx.arc(p.x, p.y, 2.5 * pulse + charge * 0.6, 0, Math.PI * 2)
    ctx.fill()
    ctx.globalAlpha = 1
  }

  if (burstActive && p.burst) {
    const r = burstAge * 700
    ctx.strokeStyle = '#1D9E75'
    ctx.lineWidth = 2
    ctx.globalAlpha = (1 - burstAge / 0.4) * 0.5
    ctx.beginPath()
    ctx.arc(p.burst.x, p.burst.y, r, 0, Math.PI * 2)
    ctx.stroke()
    ctx.globalAlpha = 1
  }
}
