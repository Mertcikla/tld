import { activePlanId } from '../api/planContext'
export function parseNumericId(value: string | null | undefined): number | null {
  if (value == null) return null
  const n = Number(value)
  return Number.isInteger(n) && (n > 0 || (n < 0 && activePlanId() !== '')) ? n : null
}

export function idToString(value: number | null | undefined): string {
  return value == null ? '' : String(value)
}
