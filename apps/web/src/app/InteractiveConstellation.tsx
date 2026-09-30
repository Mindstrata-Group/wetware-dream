'use client'

import { useEffect, useRef } from 'react'

type Particle = {
  x: number
  y: number
  vx: number
  vy: number
  seed: number
}

type PointerMode = 'attract' | 'repel' | null

export function InteractiveConstellation() {
  const ref = useRef<HTMLCanvasElement | null>(null)

  useEffect(() => {
    const canvasEl = ref.current
    if (!canvasEl) return
    const c = canvasEl
    const ctxEl = c.getContext('2d')
    if (!ctxEl) return
    const x = ctxEl

    const dpr = Math.max(1, window.devicePixelRatio || 1)
    let W = 0
    let H = 0
    let t = 0
    let raf = 0
    const pointer = { x: 0, y: 0, active: false, mode: null as PointerMode }

    const pts: Particle[] = Array.from({ length: 190 }, (_, i) => ({
      x: 0,
      y: 0,
      vx: 0,
      vy: 0,
      seed: i * 0.618,
    }))

    function resize() {
      W = window.innerWidth
      H = window.innerHeight
      c.width = Math.floor(W * dpr)
      c.height = Math.floor(H * dpr)
      c.style.width = `${W}px`
      c.style.height = `${H}px`
      x.setTransform(dpr, 0, 0, dpr, 0, 0)
      pts.forEach((p) => {
        p.x = Math.random() * W
        p.y = Math.random() * H
        const a = Math.random() * Math.PI * 2
        const s = 0.08 + Math.random() * 0.08
        p.vx = Math.cos(a) * s
        p.vy = Math.sin(a) * s
      })
    }

    function wrap(p: Particle) {
      if (p.x < -6) p.x = W + 6
      if (p.x > W + 6) p.x = -6
      if (p.y < -6) p.y = H + 6
      if (p.y > H + 6) p.y = -6
    }

    function tick() {
      t += 0.001
      x.clearRect(0, 0, W, H)

      for (let i = 0; i < pts.length; i += 1) {
        const p = pts[i]
        const a = p.seed * 9.7 + t * (0.7 + (p.seed % 0.3))
        p.vx += Math.cos(a) * 0.002
        p.vy += Math.sin(a * 1.17) * 0.002

        if (pointer.active && pointer.mode) {
          const dx = pointer.x - p.x
          const dy = pointer.y - p.y
          const d = Math.hypot(dx, dy) + 0.001
          const near = Math.max(0, 210 - d)
          const f = near * 0.000018
          const s = pointer.mode === 'attract' ? 1 : -1
          p.vx += s * (dx / d) * f
          p.vy += s * (dy / d) * f
        }

        p.vx *= 0.992
        p.vy *= 0.992
        const sp = Math.hypot(p.vx, p.vy)
        const mx = 0.28
        if (sp > mx) {
          p.vx = (p.vx / sp) * mx
          p.vy = (p.vy / sp) * mx
        }
        p.x += p.vx
        p.y += p.vy
        wrap(p)
      }

      for (let i = 0; i < pts.length; i += 1) {
        const A = pts[i]
        for (let j = i + 1; j < Math.min(pts.length, i + 18); j += 1) {
          const B = pts[j]
          const dx = A.x - B.x
          const dy = A.y - B.y
          const d = Math.hypot(dx, dy)
          if (d < 62) {
            const al = (1 - d / 62) * 0.14
            x.strokeStyle = `rgba(38,122,134,${al})`
            x.lineWidth = 1
            x.beginPath()
            x.moveTo(A.x, A.y)
            x.lineTo(B.x, B.y)
            x.stroke()
          }
        }
      }

      pts.forEach((p) => {
        x.fillStyle = 'rgba(23,167,161,0.45)'
        x.beginPath()
        x.arc(p.x, p.y, 1.6, 0, Math.PI * 2)
        x.fill()
      })

      raf = window.requestAnimationFrame(tick)
    }

    const onResize = () => resize()
    const onMove = (e: PointerEvent) => {
      pointer.x = e.clientX
      pointer.y = e.clientY
      pointer.active = true
    }
    const onLeave = () => {
      pointer.active = false
      pointer.mode = null
    }
    const onDown = (e: PointerEvent) => {
      if (e.button === 0) pointer.mode = 'attract'
      if (e.button === 2) pointer.mode = 'repel'
    }
    const onUp = () => {
      pointer.mode = null
    }
    const onContextMenu = (e: MouseEvent) => e.preventDefault()

    window.addEventListener('resize', onResize)
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerleave', onLeave)
    window.addEventListener('pointerdown', onDown)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('contextmenu', onContextMenu)

    resize()
    tick()

    return () => {
      window.cancelAnimationFrame(raf)
      window.removeEventListener('resize', onResize)
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerleave', onLeave)
      window.removeEventListener('pointerdown', onDown)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('contextmenu', onContextMenu)
    }
  }, [])

  return (
    <>
      <canvas ref={ref} className="ms-constellation-canvas" />
    </>
  )
}
