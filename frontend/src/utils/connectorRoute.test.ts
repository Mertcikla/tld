import { describe, expect, it } from 'vitest'
import { buildViewConnectorPath, connectorBezierGeometry, positionForHandleSide } from './connectorRoute'
import type { LogicalHandleSide } from './edgeDistribution'

const sides: LogicalHandleSide[] = ['left', 'right', 'top', 'bottom']

describe('shared editor and Explore curves', () => {
  it.each(sides.flatMap(source => sides.map(target => [source, target] as const)))('uses the editor cubic for %s → %s handles', (sourceSide, targetSide) => {
    const source = { x: 10, y: 600, side: sourceSide }
    const target = { x: 40, y: 20, side: targetSide }
    const sourceSize = { width: 180, height: 85 }, targetSize = { width: 180, height: 85 }
    const { control1, control2 } = connectorBezierGeometry(source, target, sourceSize, targetSize)
    const editor = buildViewConnectorPath({ routeStyle: 'bezier', sourceX: source.x, sourceY: source.y, targetX: target.x, targetY: target.y,
      sourcePosition: positionForHandleSide(sourceSide), targetPosition: positionForHandleSide(targetSide),
      sourceWidth: sourceSize.width, sourceHeight: sourceSize.height, targetWidth: targetSize.width, targetHeight: targetSize.height })
    // A single cubic, with no Explore-specific straight approaches or zoom offsets.
    expect(editor.path).toBe(`M 10,600 C ${control1.x},${control1.y} ${control2.x},${control2.y} 40,20`)
    expect(editor.labelX).toBe(0.125 * source.x + 0.375 * control1.x + 0.375 * control2.x + 0.125 * target.x)
    expect(editor.labelY).toBe(0.125 * source.y + 0.375 * control1.y + 0.375 * control2.y + 0.125 * target.y)
  })

  it.each([0.01, 0.1, 1, 4])('preserves the editor curve shape under scaling by %s', scale => {
    const source = { x: 0, y: 600 * scale, side: 'right' as const }
    const target = { x: 30 * scale, y: 0, side: 'left' as const }
    const size = { width: 180 * scale, height: 85 * scale }
    const { control1, control2 } = connectorBezierGeometry(source, target, size, size)
    expect(control1.x / scale).toBeCloseTo(90)
    expect(control1.y / scale).toBeCloseTo(600)
    expect(control2.x / scale).toBeCloseTo(-60)
    expect(control2.y).toBe(0)
  })
})
