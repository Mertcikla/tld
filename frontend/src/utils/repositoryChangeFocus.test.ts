import { describe, expect, it, vi } from 'vitest'
import type { RepositoryImpactScene } from '../api/client'
import { focusMapChanges } from './repositoryChangeFocus'
import type { MapChangeTarget } from './repositoryChangeHistory'

const scene = {
  fallbackViewId: 1,
  views: {
    1: { placements: [{ element_id: 10 }], connectors: [{ id: 30, source_element_id: 10, target_element_id: 20 }] },
    2: { placements: [{ element_id: 20 }], connectors: [] },
  },
  connectorTargets: { 'connector|1|30': { viewId: 2, elementId: 20 } },
} as unknown as RepositoryImpactScene
const target = (kind: MapChangeTarget['kind'], viewId = 1): MapChangeTarget => ({ key: kind === 'view' ? `view|${viewId}` : `${kind === 'element' ? 'placement' : kind}|${viewId}|${kind === 'element' ? 10 : 30}`, kind, viewId, elementIds: kind === 'view' ? [] : [{ viewId, elementId: 10 }], ancestorViewIds: [1] })
const camera = () => ({ focusElement: vi.fn(() => true), focusElements: vi.fn(() => true), focusDiagram: vi.fn(() => true) })

describe('history navigation', () => {
  it('focuses an exact element and resolves an element that moved to another view', () => {
    const canvas = camera()
    expect(focusMapChanges(scene, [target('element')], canvas)).toBe(true)
    expect(canvas.focusElement).toHaveBeenLastCalledWith(1, 10)
    const moved = { ...target('element'), elementIds: [{ viewId: 1, elementId: 20 }] }
    focusMapChanges(scene, [moved], canvas)
    expect(canvas.focusElement).toHaveBeenLastCalledWith(2, 20)
  })

  it('frames both ends of a cross-view connector', () => {
    const canvas = camera()
    focusMapChanges(scene, [target('connector')], canvas)
    expect(canvas.focusElements).toHaveBeenCalledWith([{ viewId: 1, elementId: 10 }, { viewId: 2, elementId: 20 }])
    expect(canvas.focusDiagram).not.toHaveBeenCalled()
  })

  it('focuses views directly and removed views through surviving ancestors', () => {
    const canvas = camera()
    focusMapChanges(scene, [target('view', 2)], canvas)
    expect(canvas.focusDiagram).toHaveBeenLastCalledWith(2)
    focusMapChanges(scene, [target('view', 99)], canvas)
    expect(canvas.focusDiagram).toHaveBeenLastCalledWith(1)
  })

  it('uses surviving endpoints for removed connectors and context for removed elements', () => {
    const canvas = camera()
    const removedConnector = { ...target('connector'), key: 'connector|1|999', elementIds: [{ viewId: 1, elementId: 10 }, { viewId: 2, elementId: 999 }] }
    focusMapChanges(scene, [removedConnector], canvas)
    expect(canvas.focusElement).toHaveBeenCalledWith(1, 10)
    const removedElement = { ...target('element'), elementIds: [{ viewId: 99, elementId: 999 }], viewId: 99 }
    focusMapChanges(scene, [removedElement], canvas)
    expect(canvas.focusDiagram).toHaveBeenCalledWith(1)
  })

  it('deduplicates endpoints across a file group and returns false until the canvas is ready', () => {
    const canvas = camera()
    focusMapChanges(scene, [target('element'), target('connector')], canvas)
    expect(canvas.focusElements).toHaveBeenCalledWith([{ viewId: 1, elementId: 10 }, { viewId: 2, elementId: 20 }])
    canvas.focusElements.mockReturnValue(false)
    canvas.focusDiagram.mockReturnValue(false)
    expect(focusMapChanges(scene, [target('connector')], canvas)).toBe(false)
  })
})
