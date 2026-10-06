export type HandleSide = 'top' | 'bottom' | 'left' | 'right'

// Matches the node dimensions used by internal/layout.ChooseConnectorHandles so
// the overlay picks the same anchors the map pipeline would.
export const CONNECTOR_HANDLE_NODE_WIDTH = 200
export const CONNECTOR_HANDLE_NODE_HEIGHT = 120

interface Point {
  x: number
  y: number
}

interface HandleAnchor {
  name: HandleSide
  x: number
  y: number
}

function handleAnchors(p: Point): HandleAnchor[] {
  return [
    { name: 'top', x: p.x + CONNECTOR_HANDLE_NODE_WIDTH / 2, y: p.y },
    { name: 'bottom', x: p.x + CONNECTOR_HANDLE_NODE_WIDTH / 2, y: p.y + CONNECTOR_HANDLE_NODE_HEIGHT },
    { name: 'left', x: p.x, y: p.y + CONNECTOR_HANDLE_NODE_HEIGHT / 2 },
    { name: 'right', x: p.x + CONNECTOR_HANDLE_NODE_WIDTH, y: p.y + CONNECTOR_HANDLE_NODE_HEIGHT / 2 },
  ]
}

// chooseConnectorHandles returns the source and target handle sides whose node
// anchor points are closest, so a connector leaves and enters its nodes on
// facing sides instead of crossing their bodies. It mirrors the map pipeline's
// layout.ChooseConnectorHandles / the editor's "Adjust Connectors" pass.
export function chooseConnectorHandles(source: Point, target: Point): { source: HandleSide; target: HandleSide } {
  let bestSource: HandleSide = 'top'
  let bestTarget: HandleSide = 'top'
  let bestDistance = Infinity
  for (const s of handleAnchors(source)) {
    for (const t of handleAnchors(target)) {
      const distance = Math.hypot(s.x - t.x, s.y - t.y)
      if (distance < bestDistance) {
        bestDistance = distance
        bestSource = s.name
        bestTarget = t.name
      }
    }
  }
  return { source: bestSource, target: bestTarget }
}
