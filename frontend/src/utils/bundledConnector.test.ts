import { describe, expect, it } from 'vitest'
import { bundleFan, bundleFans, bundlePathPolylines, closestBundleHandle } from './bundledConnector'

it('chooses the nearer side without dropping any declaration lines', () => {
  expect(closestBundleHandle('right-line-12,18,26', 400, 100)).toBe('left-line-12,18,26')
  expect(closestBundleHandle('left-line-12,18,26', 100, 400)).toBe('right-line-12,18,26')
  expect(closestBundleHandle('left-symbol-save', 100, 400)).toBe('left-symbol-save')
  expect(closestBundleHandle('left-line-12', 100, 400)).toBe('left-line-12')
  expect(closestBundleHandle('left-line-12', 100, 400, true)).toBe('right-line-12')
  expect(closestBundleHandle('left-line-12,18', 100, 100)).toBe('left-line-12,18')
})

describe('smoothstep bundle fan', () => {
  it('splits near the card and deduplicates decorator aliases and overflow ports', () => {
    const fan = bundleFan([
      { x: 100, y: 60, side: 'left' },
      { x: 100, y: 20, side: 'left' },
      { x: 100, y: 20, side: 'left' },
    ])!
    expect(fan).toMatchObject({ x: 68, y: 40, side: 'left' })
    expect(fan.branches).toHaveLength(2)
    expect(fan.branches.some(path => path.includes('Q'))).toBe(true)
    for (const path of fan.branches) expect(path).not.toContain('C')
    expect(fan.branches[0]).toMatch(/^M\s*100[ ,]20/)
    const target = bundleFan([{ x: 100, y: 20, side: 'right' }, { x: 100, y: 60, side: 'right' }], true)!
    expect(target).toMatchObject({ x: 132, y: 40 })
    expect(target.branches[0]).toMatch(/^M\s*116[ ,]40/)
    expect(target.stems).toEqual(['M 132,40 L 116,40'])
  })

  it('keeps collapsed bundles on a single port and ignores unmounted overview aliases', () => {
    expect(bundleFan([])).toBeNull()
    expect(bundleFan([{ x: 100, y: 20, side: 'left' }, { x: 100, y: 20, side: 'left' }]))
      .toEqual({ x: 100, y: 20, side: 'left', branches: [], stems: [] })
  })
})

it('staggers neighboring forks and bridges crossings without depending on edge order', () => {
  const inputs = [
    { key: '10:source', ports: [{ x: 100, y: 0, side: 'right' as const }, { x: 100, y: 100, side: 'right' as const }] },
    { key: '20:target', target: true, ports: [{ x: 100, y: 20, side: 'right' as const }, { x: 100, y: 140, side: 'right' as const }] },
  ]
  const fans = bundleFans(inputs)
  const first = fans.get('10:source')!, second = fans.get('20:target')!
  expect(first.branches[0]).toContain('116,')
  expect(second.branches[0]).toContain('140,')
  // Trunks start beyond both rails, keeping their curves out of the fork area.
  expect(first.x).toBe(156)
  expect(second.x).toBe(156)
  expect([...first.branches, ...first.stems].join(' ')).toContain('140,46')
  expect([...second.branches, ...second.stems].join(' ')).toContain('116,16')
  // A crossing cuts the underneath rail rather than drawing a false junction.
  expect([...first.branches, ...first.stems].some(path => (path.match(/M /g) ?? []).length > 1)).toBe(true)
  expect([...second.branches, ...second.stems].some(path => (path.match(/M /g) ?? []).length > 1)).toBe(true)
  expect(bundleFans([...inputs].reverse())).toEqual(fans)
})

it('separates coincident trunk heights, avoids peer port lines, and gives each card side independent lanes', () => {
  const ports = [{ x: 100, y: 0, side: 'right' as const }, { x: 100, y: 100, side: 'right' as const }]
  const fans = bundleFans([
    { key: 'a', ports }, { key: 'b', ports },
    { key: 'c', ports: ports.map(port => ({ ...port, side: 'left' as const })) },
    { key: 'collapsed', ports: [{ x: 100, y: 50, side: 'right' as const }] },
  ])
  expect(fans.get('a')!.y).not.toBe(50)
  expect(fans.get('b')!.y).not.toBe(fans.get('a')!.y)
  expect(fans.get('c')!.x).toBe(68)
  expect(fans.get('collapsed')!.branches).toEqual([])
})

it('samples rounded bridge curves for picking without joining the gaps underneath', () => {
  const paths = bundlePathPolylines('M 0,0 L 10,0 Q 10,-4 14,-4 Q 18,-4 18,0 L 30,0 M 30,12 L 30,30')
  expect(paths).toHaveLength(2)
  expect(paths[0].some(point => point.y === -4)).toBe(true)
  expect(paths[0].at(-1)).toEqual({ x: 30, y: 0 })
  expect(paths[1]).toEqual([{ x: 30, y: 12 }, { x: 30, y: 30 }])
})

it('paints every shared rail and stem only once, including during hover animation', () => {
  const fan = bundleFan([0, 50, 100, 200].map(y => ({ x: 100, y, side: 'right' as const })))!
  const paths = [...fan.branches, ...fan.stems]
  const segments = paths.flatMap((path, owner) => bundlePathPolylines(path).flatMap(points => points.slice(1).map((b, index) => ({ owner, a: points[index], b }))))
  for (const a of segments) for (const b of segments) {
    if (a.owner >= b.owner) continue
    if (a.a.x === a.b.x && b.a.x === b.b.x && a.a.x === b.a.x) {
      const overlap = Math.min(Math.max(a.a.y, a.b.y), Math.max(b.a.y, b.b.y)) - Math.max(Math.min(a.a.y, a.b.y), Math.min(b.a.y, b.b.y))
      expect(overlap).toBeLessThanOrEqual(0)
    }
    if (a.a.y === a.b.y && b.a.y === b.b.y && a.a.y === b.a.y) {
      const overlap = Math.min(Math.max(a.a.x, a.b.x), Math.max(b.a.x, b.b.x)) - Math.max(Math.min(a.a.x, a.b.x), Math.min(b.a.x, b.b.x))
      expect(overlap).toBeLessThanOrEqual(0)
    }
  }
  expect(paths.every(path => bundlePathPolylines(path).length === 1)).toBe(true)
})

it('limits crossing gaps to the bridge crest and keeps vertical subpaths monotonic in both directions', () => {
  const fans = bundleFans([
    { key: 'a', ports: [{ x: 100, y: 0, side: 'right' }, { x: 100, y: 60, side: 'right' }, { x: 100, y: 120, side: 'right' }] },
    { key: 'b', target: true, ports: [{ x: 100, y: 20, side: 'right' }, { x: 100, y: 100, side: 'right' }, { x: 100, y: 160, side: 'right' }] },
  ])
  let gaps = 0
  for (const fan of fans.values()) for (const path of [...fan.branches, ...fan.stems]) {
    const routes = bundlePathPolylines(path)
    for (let i = 1; i < routes.length; i++) {
      const before = routes[i - 1].at(-1)!, after = routes[i][0]
      expect(Math.hypot(after.x - before.x, after.y - before.y)).toBeLessThanOrEqual(3)
      gaps++
    }
  }
  expect(gaps).toBeGreaterThan(0)
})
