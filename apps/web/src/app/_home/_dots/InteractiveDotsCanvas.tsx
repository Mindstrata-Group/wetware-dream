'use client'

import { useEffect, useRef } from 'react'
import { HP, makeDot } from './physics'
import { installPointerHandlers } from './interaction'
import { renderConnections, renderDots, renderPointerFeedback } from './render'
import type { Dot, PointerState } from './types'

export function InteractiveDotsCanvas() {
  const ref = useRef<HTMLCanvasElement>(null)
  // pointer/interaction state shared with rAF loop via ref (no re-renders)
  const ptr = useRef<PointerState>({
    x: 0,
    y: 0,
    hovering: false,
    holding: false,
    holdT0: 0,
    // burst = explosion in progress after release
    burst: null as null | { x: number; y: number; t0: number; strength: number },
  })

  useEffect(() => {
    const c = ref.current
    if (!c) return
    const ctx = c.getContext('2d')
    if (!ctx) return
    const parent = c.parentElement!

    let dots: Dot[] = []
    let raf = 0

    const setup = () => {
      const rect = parent.getBoundingClientRect()
      const dpr = Math.min(window.devicePixelRatio || 1, 2)
      c.width = rect.width * dpr
      c.height = rect.height * dpr
      c.style.width = rect.width + 'px'
      c.style.height = rect.height + 'px'
      ctx.setTransform(1, 0, 0, 1, 0, 0)
      ctx.scale(dpr, dpr)
      // denser, livelier baseline than before
      const count = Math.min(140, Math.max(34, Math.round((rect.width * rect.height) / 9500)))
      dots = Array.from({ length: count }, () => makeDot(rect.width, rect.height))
    }

    const tick = () => {
      const rect = parent.getBoundingClientRect()
      ctx.clearRect(0, 0, rect.width, rect.height)
      const now = performance.now()
      const p = ptr.current

		// ── global interaction cycle ─────────────────────────────────────
		// first 30 seconds: pure drift
		// next 30 seconds: particle interactions
		const cycleMs = HP.cleanDriftMs + HP.interactionMs
		const cycleAge = now % cycleMs
		const interactionsEnabled = cycleAge >= HP.cleanDriftMs

      // ── phase detection ─────────────────────────────────────────────────
      const burstAge = p.burst ? (now - p.burst.t0) / 1000 : Infinity
      const burstActive = !!p.burst && burstAge < 0.4
      const settling   = !!p.burst && burstAge >= 0.4 && burstAge < 1.4
      if (p.burst && burstAge >= 1.4) {
        p.burst = null
        // resume natural drift after settle phase
        for (const d of dots) {
		  if (!d.visible) continue
		    d.vx = (Math.random() - 0.5) * HP.resetV
			d.vy = (Math.random() - 0.5) * HP.resetV
		}
      }

		// ── respawn invisible dots ───────────────────────────────────────
		for (let i = 0; i < dots.length; i++) {
		  const d = dots[i]

		  if (!d.visible && now >= d.respawnAt) {
			dots[i] = makeDot(rect.width, rect.height)
		  }
		}

		// ── temporary antiparticles return to normal ─────────────────────
		for (const d of dots) {
		  if (!d.visible) continue

		  if (d.antiUntil > 0 && now >= d.antiUntil) {
			d.kind = d.baseKind
			d.charge = d.baseCharge
			d.antiUntil = 0
		  }
		}

		// ── mouse attraction: slower and ignores invisible dots ─────────────
		if (p.holding) {
		  const heldSec = (now - p.holdT0) / 1000
		  const charge = 1 + Math.log(1 + heldSec) * HP.holdChargeGrow
		  const reach = HP.holdReach
		  const factor = HP.holdFactor * charge

		  for (const d of dots) {
			if (!d.visible) continue

			const dx = p.x - d.x
			const dy = p.y - d.y
			const dist = Math.hypot(dx, dy) + 0.001

			if (dist < reach) {
			  const f = (reach - dist) * factor
			  d.vx += (dx / dist) * f
			  d.vy += (dy / dist) * f
			}
		  }
		} else if (p.hovering && !burstActive && !settling) {
		  // gentle hover attraction, slower
		  for (const d of dots) {
			if (!d.visible) continue

			const dx = p.x - d.x
			const dy = p.y - d.y
			const dist = Math.hypot(dx, dy) + 0.001

			if (dist < HP.hoverReach) {
			  const f = (HP.hoverReach - dist) * HP.hoverFactor
			  d.vx += (dx / dist) * f
			  d.vy += (dy / dist) * f
			}
		  }
		}

		// ── burst: explosion impulse, ignores invisible dots ───────────────
		if (burstActive && p.burst) {
		  const fade = 1 - burstAge / 0.4
		  const s = p.burst.strength * fade

		  for (const d of dots) {
			if (!d.visible) continue

			const dx = d.x - p.burst.x
			const dy = d.y - p.burst.y
			const dist = Math.hypot(dx, dy) + 0.001

			const f = (s / Math.max(dist * 0.06, 1.8)) * 0.045
			d.vx += (dx / dist) * f
			d.vy += (dy / dist) * f
		  }
		}

		// ── particle-to-particle field ────────────────────────────────────
		// an attractor pulls neighbouring points
		// a repeller pushes neighbouring points away
		// neutral creates nothing itself but can move away from another field
		if (interactionsEnabled && !p.holding && !burstActive && !settling) {
		  const fieldReach = HP.fieldReach
		  const fieldStrength = HP.fieldStrength

		  for (let i = 0; i < dots.length; i++) {
			const a = dots[i]
			if (!a.visible) continue

			for (let j = i + 1; j < dots.length; j++) {
			  const b = dots[j]
			  if (!b.visible) continue

			  const dx = b.x - a.x
			  const dy = b.y - a.y
			  const dist = Math.hypot(dx, dy) + 0.001

			  if (dist < fieldReach) {
				const ux = dx / dist
				const uy = dy / dist
				const proximity = (fieldReach - dist) / fieldReach
				const f = proximity * proximity * fieldStrength

				// the field of point A acts on B
				if (a.charge !== 0) {
				  b.vx -= ux * f * a.charge
				  b.vy -= uy * f * a.charge
				}

				// the field of point B acts on A
				if (b.charge !== 0) {
				  a.vx += ux * f * b.charge
				  a.vy += uy * f * b.charge
				}
			  }
			}
		  }
		}

// ── hard group breaker: 5+ linked dots create temporary antiparticle ──
if (interactionsEnabled && !p.holding && !burstActive && !settling) {
  const visited = new Array(dots.length).fill(false)

  for (let start = 0; start < dots.length; start++) {
    if (visited[start]) continue

    const first = dots[start]
    if (!first.visible) {
      visited[start] = true
      continue
    }

    const group: number[] = []
    const stack = [start]
    visited[start] = true

    // find a visually connected group using the same radius that lines are drawn with
    while (stack.length > 0) {
      const i = stack.pop()!
      const a = dots[i]
      group.push(i)

      for (let j = 0; j < dots.length; j++) {
        if (visited[j]) continue

        const b = dots[j]
        if (!b.visible) {
          visited[j] = true
          continue
        }

        const dx = b.x - a.x
        const dy = b.y - a.y
        const dist = Math.hypot(dx, dy)

        if (dist < HP.lineReach) {
          visited[j] = true
          stack.push(j)
        }
      }
    }

    // 4 points are fine. From the 5th on, the group counts as overloaded.
    if (group.length <= HP.maxVisualGroupSize) continue

    // if the group was broken up recently, do not repeat it every frame
    const inCooldown = group.some(idx => now < dots[idx].breakCooldownUntil)
    if (inCooldown) continue

    // group centre
    let cx = 0
    let cy = 0
    for (const idx of group) {
      cx += dots[idx].x
      cy += dots[idx].y
    }
    cx /= group.length
    cy /= group.length

    // pick a random particle inside the group and make it an antiparticle for 5 seconds
    const pick = group[Math.floor(Math.random() * group.length)]
    const anti = dots[pick]

    anti.kind = 'repeller'
    anti.charge = HP.antiCharge
    anti.antiUntil = now + HP.antiDurationMs

    // shared cooldown for the group
    for (const idx of group) {
      dots[idx].breakCooldownUntil = now + HP.clusterBreakCooldownMs
    }

    // a strong kick to the chosen antiparticle from the group centre
    let dx = anti.x - cx
    let dy = anti.y - cy
    let dist = Math.hypot(dx, dy)

    if (dist < 0.001) {
      const angle = Math.random() * Math.PI * 2
      dx = Math.cos(angle)
      dy = Math.sin(angle)
      dist = 1
    }

    anti.vx += (dx / dist) * HP.breakImpulse
    anti.vy += (dy / dist) * HP.breakImpulse

    // gently spreads the whole group outwards
    for (const idx of group) {
      const d = dots[idx]

      let gx = d.x - cx
      let gy = d.y - cy
      let gd = Math.hypot(gx, gy)

      if (gd < 0.001) {
        const angle = Math.random() * Math.PI * 2
        gx = Math.cos(angle)
        gy = Math.sin(angle)
        gd = 1
      }

      d.vx += (gx / gd) * HP.groupImpulse
      d.vy += (gy / gd) * HP.groupImpulse
    }
  }
}

	// ── anti-clump: if more than 3 dots gather, split them apart ──────
	if (interactionsEnabled && !p.holding && !burstActive && !settling) {
		const personalSpace = HP.personalSpace
		const clusterRadius = HP.clusterRadius
		const maxClusterSize = HP.maxClusterSize
		const overlapStrength = HP.overlapStrength
		const splitStrength = HP.splitStrength

	  for (let i = 0; i < dots.length; i++) {
		const a = dots[i]
		if (!a.visible) continue

		let nearCount = 0
		let cx = a.x
		let cy = a.y

		for (let j = 0; j < dots.length; j++) {
		  if (i === j) continue

		  const b = dots[j]
		  if (!b.visible) continue

		  const dx = b.x - a.x
		  const dy = b.y - a.y
		  const dist = Math.hypot(dx, dy) + 0.001

		  // 1) Hard personal space:
		  // if points are too close, push them apart right away
		  if (dist < personalSpace) {
			const ux = dx / dist
			const uy = dy / dist
			const f = (personalSpace - dist) * overlapStrength

			a.vx -= ux * f
			a.vy -= uy * f
		  }

		  // 2) Compute the local cluster
		  if (dist < clusterRadius) {
			nearCount++
			cx += b.x
			cy += b.y
		  }
		}

		// if there are 3 neighbours nearby, then there are already 4+ points together
		if (nearCount >= maxClusterSize) {
		  cx /= (nearCount + 1)
		  cy /= (nearCount + 1)

		  let dx = a.x - cx
		  let dy = a.y - cy
		  let dist = Math.hypot(dx, dy)

		  // if a point is almost exactly in the cluster centre, give it a random push
		  if (dist < 0.001) {
			const angle = Math.random() * Math.PI * 2
			dx = Math.cos(angle)
			dy = Math.sin(angle)
			dist = 1
		  }

		  const excess = nearCount - maxClusterSize + 1
		  const f = splitStrength * (1 + excess * 0.8)

		  a.vx += (dx / dist) * f
		  a.vy += (dy / dist) * f
		}
	  }
	}
      // ── damping & speed cap by phase ───────────────────────────────────
      // - holding: low damping, high cap → dots accelerate freely toward center
      // - burst:   medium damping, very high cap → dots fly outward
      // - settle:  HIGH damping → energy bleeds out like billiard balls
      // - normal:  low damping → lively drift
		let damp: number = HP.dampNormal
		let speedCap: number = HP.speedNormal

		if (p.holding)   { damp = HP.dampHolding; speedCap = HP.speedHolding }
		if (burstActive) { damp = HP.dampBurst; speedCap = HP.speedBurst }
		if (settling)    { damp = HP.dampSettling; speedCap = HP.speedSettling }

      // Natural drift: tiny Brownian jitter keeps motion alive in calm phases
      // (off during hold/burst/settle so the dramatic effects stay clean)
      const driftJitter = (!p.holding && !burstActive && !settling)
		  ? (interactionsEnabled ? HP.driftJitter : HP.cleanDriftJitter)
		  : 0
	  
      for (const d of dots) {
		  if (!d.visible) continue

		  if (driftJitter > 0) {
          d.vx += (Math.random() - 0.5) * driftJitter
          d.vy += (Math.random() - 0.5) * driftJitter
        }
        d.x += d.vx
        d.y += d.vy
        // wrap
        if (d.x < -6) d.x = rect.width + 6
        if (d.x > rect.width + 6) d.x = -6
        if (d.y < -6) d.y = rect.height + 6
        if (d.y > rect.height + 6) d.y = -6
        d.vx *= damp
        d.vy *= damp
        const sp = Math.hypot(d.vx, d.vy)
        if (sp > speedCap) {
          d.vx = (d.vx / sp) * speedCap
          d.vy = (d.vy / sp) * speedCap
        }
      }

      const neighborCounts = renderConnections(ctx, dots, now, interactionsEnabled)

      // ── lonely dots can disappear and respawn later ────────────────────
      if (interactionsEnabled && !p.holding && !burstActive && !settling) {
        for (let i = 0; i < dots.length; i++) {
          const d = dots[i]
          if (!d.visible) continue

          if (neighborCounts[i] === 0 && Math.random() < HP.lonelyDisappearChance) {
            d.visible = false
            d.respawnAt = now + HP.respawnMinMs + Math.random() * HP.respawnRandomMs
          }
        }
      }

      renderDots(ctx, dots)
      renderPointerFeedback(ctx, p, now, burstAge, burstActive)
      raf = requestAnimationFrame(tick)
    }

    const cleanupPointerHandlers = installPointerHandlers(c, ptr)

    setup()
    tick()
    const ro = new ResizeObserver(() => setup())
    ro.observe(parent)

    return () => {
      cancelAnimationFrame(raf)
      ro.disconnect()
      cleanupPointerHandlers()
    }
  }, [])

  return (
    <canvas
      ref={ref}
      style={{
        position: 'absolute', inset: 0,
        width: '100%', height: '100%',
        // canvas never blocks UI; we listen at window level for events
        pointerEvents: 'none',
        zIndex: 0,
      }}
    />
  )
}

