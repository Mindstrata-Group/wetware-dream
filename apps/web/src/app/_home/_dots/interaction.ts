import type { MutableRefObject } from 'react'
import type { PointerState } from './types'

export function installPointerHandlers(c: HTMLCanvasElement, ptr: MutableRefObject<PointerState>) {
  const toLocal = (clientX: number, clientY: number) => {
    const rect = c.getBoundingClientRect()
    return { x: clientX - rect.left, y: clientY - rect.top }
  }

  const onMove = (e: PointerEvent) => {
    const { x, y } = toLocal(e.clientX, e.clientY)
    ptr.current.x = x
    ptr.current.y = y
    ptr.current.hovering = true
  }

  const onLeaveWindow = () => {
    ptr.current.hovering = false
  }

  const onDown = (e: PointerEvent) => {
    const { x, y } = toLocal(e.clientX, e.clientY)
    ptr.current.x = x
    ptr.current.y = y
    ptr.current.hovering = true
    ptr.current.holding = true
    ptr.current.holdT0 = performance.now()
    ptr.current.burst = null
  }

  const onUp = () => {
    const p = ptr.current
    if (!p.holding) return
    const heldSec = (performance.now() - p.holdT0) / 1000
    const strength = 260 + Math.log(1 + heldSec * 5) * 460
    p.burst = { x: p.x, y: p.y, t0: performance.now(), strength }
    p.holding = false
  }

  const onContextMenu = (e: MouseEvent) => {
    if (ptr.current.holding) e.preventDefault()
  }

  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerdown', onDown)
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', onUp)
  window.addEventListener('contextmenu', onContextMenu)
  window.addEventListener('blur', onLeaveWindow)
  document.addEventListener('mouseleave', onLeaveWindow)

  return () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerdown', onDown)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onUp)
    window.removeEventListener('contextmenu', onContextMenu)
    window.removeEventListener('blur', onLeaveWindow)
    document.removeEventListener('mouseleave', onLeaveWindow)
  }
}
