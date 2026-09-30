import { apiBase } from './api'

export type PublicTariffMode = {
  id: number;
  name: string;
  welcomeMessage?: string;
}

export type PublicTariff = {
  id: number;
  name: string;
  description: string;
  tariffType: string;
  monthlyPrice: number;
  dailyMessageLimit: number;
  limitType: string;
  groupName: string;
  modeIds: number[];
  modes: PublicTariffMode[];
}

let publicTariffsMemo: { data: PublicTariff[]; expiresAt: number } | null = null
const PUBLIC_TARIFFS_CACHE_MS = 60_000

export async function loadPublicTariffs(): Promise<PublicTariff[]> {
  const now = Date.now()
  if (publicTariffsMemo && now < publicTariffsMemo.expiresAt) {
    return publicTariffsMemo.data
  }
  try {
    const res = await fetch(`${apiBase}/api/public/tariffs`)
    if (!res.ok) return publicTariffsMemo?.data ?? []
    const json = await res.json()
    const data = Array.isArray(json?.tariffs) ? json.tariffs : []
    publicTariffsMemo = { data, expiresAt: now + PUBLIC_TARIFFS_CACHE_MS }
    return data
  } catch {
    return publicTariffsMemo?.data ?? []
  }
}

export function clearPublicTariffsCacheForTests() {
  publicTariffsMemo = null
}

export function formatRub(value: number): string {
  return `${Math.round(value).toLocaleString('ru-RU')} ₽`
}
