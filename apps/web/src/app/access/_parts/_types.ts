export type AccessStatusResponse = {
  ok: boolean
  hasAccess: boolean
}

export type ApplyResponse = {
  ok: boolean
  error?: string
  code?: string
  errorCode?: string
  accessDays?: number
  modes?: Array<{ modeId?: number; modeName?: string; activeTo?: string }>
  grantedModes?: Array<{ modeId?: number; modeName?: string; activeTo?: string }>
  extendedModes?: Array<{ modeId?: number; modeName?: string; activeTo?: string }>
  roleUpgraded?: boolean
  role?: string
  requiresAuth?: boolean
}

export type PromoInfo = {
  code: string
  accessDays?: number
  modes: Array<{ modeId?: number; name: string; activeTo?: string }>
  grantedModes: Array<{ modeId?: number; name: string; activeTo?: string }>
  extendedModes: Array<{ modeId?: number; name: string; activeTo?: string }>
  modeIds: number[]
  roleUpgraded?: boolean
}
