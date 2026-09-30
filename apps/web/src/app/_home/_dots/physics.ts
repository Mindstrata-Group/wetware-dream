import type { Dot, DotKind } from './types'

export const HP = {
  baseV: 0.32,
  resetV: 0.28,
  driftJitter: 0.012,
  cleanDriftJitter: 0.010,
  cleanDriftMs: 30000,
  interactionMs: 30000,
  holdReach: 620,
  holdFactor: 0.000055,
  holdChargeGrow: 0.9,
  hoverReach: 340,
  hoverFactor: 0.000010,
  fieldReach: 125,
  fieldStrength: 0.0024,
  personalSpace: 11,
  clusterRadius: 22,
  maxClusterSize: 3,
  overlapStrength: 0.028,
  splitStrength: 0.014,
  lineReach: 92,
  maxVisualGroupSize: 4,
  antiDurationMs: 2600,
  clusterBreakCooldownMs: 2200,
  antiCharge: -1.65,
  breakImpulse: 0.24,
  groupImpulse: 0.018,
  antiLineAlpha: 0.65,
  maxLinksPerDot: 5,
  dampNormal: 0.984,
  dampHolding: 0.955,
  dampBurst: 0.945,
  dampSettling: 0.82,
  speedNormal: 1.05,
  speedHolding: 3.8,
  speedBurst: 11,
  speedSettling: 5.5,
  lonelyDisappearChance: 0.00018,
  respawnMinMs: 2200,
  respawnRandomMs: 3200,
} as const

export const pickDotKind = (): DotKind => {
  const roll = Math.random()
  if (roll < 0.15) return 'attractor'
  if (roll < 0.30) return 'repeller'
  return 'neutral'
}

export const makeDot = (width: number, height: number): Dot => {
  const kind = pickDotKind()
  const charge = kind === 'attractor' ? 1 : kind === 'repeller' ? -1 : 0

  return {
    x: Math.random() * width,
    y: Math.random() * height,
    vx: (Math.random() - 0.5) * 0.31,
    vy: (Math.random() - 0.5) * 0.31,
    r: Math.random() * 1.6 + 0.5,
    a: 0.35 + Math.random() * 0.55,
    kind,
    charge,
    baseKind: kind,
    baseCharge: charge,
    antiUntil: 0,
    breakCooldownUntil: 0,
    visible: true,
    respawnAt: 0,
  }
}
