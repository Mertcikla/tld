import { api } from '../api/client'

export const NODE_W = 200
export const NODE_H = 120
export const PADDING = 40

export async function removeCollisions(viewId: number) {
  const [objs, edgeList] = await Promise.all([
    api.workspace.views.placements.list(viewId),
    api.workspace.connectors.list(viewId),
  ])

  const newPositions = new Map<number, { x: number; y: number }>()
  objs.forEach((o) => newPositions.set(o.element_id, { x: o.position_x, y: o.position_y }))

  for (let pass = 0; pass < 3; pass++) {
    for (let j = 0; j < objs.length; j++) {
      for (let k = j + 1; k < objs.length; k++) {
        const a = objs[j]
        const b = objs[k]
        const posA = newPositions.get(a.element_id)!
        const posB = newPositions.get(b.element_id)!

        const dx = posB.x + NODE_W / 2 - (posA.x + NODE_W / 2)
        const dy = posB.y + NODE_H / 2 - (posA.y + NODE_H / 2)
        const adx = Math.abs(dx)
        const ady = Math.abs(dy)

        const minX = NODE_W + PADDING
        const minY = NODE_H + PADDING

        if (adx < minX && ady < minY) {
          const pushX = (minX - adx) / 2
          const pushY = (minY - ady) / 2
          const factorX = dx >= 0 ? 1 : -1
          const factorY = dy >= 0 ? 1 : -1

          posA.x -= pushX * factorX
          posA.y -= pushY * factorY
          posB.x += pushX * factorX
          posB.y += pushY * factorY
        }
      }
    }
  }

  await Promise.all(
    objs.map((obj) => {
      const pos = newPositions.get(obj.element_id)!
      return api.workspace.views.placements.updatePosition(
        viewId,
        obj.element_id,
        Math.round(pos.x),
        Math.round(pos.y)
      )
    })
  )

  const handleUpdates = []
  for (const edge of edgeList) {
    const sPos = newPositions.get(edge.source_element_id)
    const tPos = newPositions.get(edge.target_element_id)
    if (!sPos || !tPos) continue

    const sourceHandles: Record<string, { x: number; y: number }> = {
      top: { x: sPos.x + NODE_W / 2, y: sPos.y },
      bottom: { x: sPos.x + NODE_W / 2, y: sPos.y + NODE_H },
      left: { x: sPos.x, y: sPos.y + NODE_H / 2 },
      right: { x: sPos.x + NODE_W, y: sPos.y + NODE_H / 2 },
    }

    const targetHandles: Record<string, { x: number; y: number }> = {
      top: { x: tPos.x + NODE_W / 2, y: tPos.y },
      bottom: { x: tPos.x + NODE_W / 2, y: tPos.y + NODE_H },
      left: { x: tPos.x, y: tPos.y + NODE_H / 2 },
      right: { x: tPos.x + NODE_W, y: tPos.y + NODE_H / 2 },
    }

    let minDist = Infinity
    let bestSource = edge.source_handle || 'top'
    let bestTarget = edge.target_handle || 'top'

    for (const [sId, sCoord] of Object.entries(sourceHandles)) {
      for (const [tId, tCoord] of Object.entries(targetHandles)) {
        const dist = Math.sqrt((sCoord.x - tCoord.x) ** 2 + (sCoord.y - tCoord.y) ** 2)
        if (dist < minDist) {
          minDist = dist
          bestSource = sId
          bestTarget = tId
        }
      }
    }

    if (bestSource !== edge.source_handle || bestTarget !== edge.target_handle) {
      handleUpdates.push(api.workspace.connectors.update(viewId, edge.id, {
        source_element_id: edge.source_element_id,
        target_element_id: edge.target_element_id,
        source_handle: bestSource,
        target_handle: bestTarget,
        label: edge.label || undefined,
        description: edge.description || undefined,
        relationship: edge.relationship || undefined,
        direction: edge.direction || undefined,
        style: edge.style === 'default' ? 'bezier' : (edge.style || 'bezier'),
        url: edge.url || undefined,
        tags: edge.tags ?? [],
      }))
    }
  }

  await Promise.all(handleUpdates)
}
