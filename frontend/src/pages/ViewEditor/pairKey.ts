import { parseNumericId } from '../../utils/ids'

export function canonicalNodePairKey(leftId: string, rightId: string) {
  const leftNumericId = parseNumericId(leftId)
  const rightNumericId = parseNumericId(rightId)

  if (leftNumericId != null && rightNumericId != null) {
    return leftNumericId <= rightNumericId
      ? `${leftId}::${rightId}`
      : `${rightId}::${leftId}`
  }

  return leftId <= rightId ? `${leftId}::${rightId}` : `${rightId}::${leftId}`
}