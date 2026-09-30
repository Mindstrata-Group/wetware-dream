export type DotKind = 'neutral' | 'attractor' | 'repeller'

export type Dot = {
  x: number
  y: number
  vx: number
  vy: number
  r: number
  a: number
  kind: DotKind
  charge: number
  baseKind: DotKind
  baseCharge: number
  antiUntil: number
  breakCooldownUntil: number
  visible: boolean
  respawnAt: number
}

export type PointerState = {
  x: number
  y: number
  hovering: boolean
  holding: boolean
  holdT0: number
  burst: null | { x: number; y: number; t0: number; strength: number }
}
