import { readFileSync } from 'node:fs'
import { create } from '@bufbuild/protobuf'
import { ImpactPlacementRefSchema, ImpactSceneSchema, ScenePlacementSchema, SceneViewContentSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/codeindex/v1/codeindex_pb.js'
import { describe, expect, it } from 'vitest'
import { impactSceneMermaid } from './impactSceneMermaid'

const cases = JSON.parse(readFileSync(new URL('../../../internal/mermaid/fixtures/connector-directions.json', import.meta.url), 'utf8')) as {
  direction: string; change: string; label: string; targetView: number; line: string
}[]

describe('comparison Mermaid connector directions', () => {
  it.each(cases)('$direction/$change preserves endpoints and arrows', ({ direction, change, label, targetView, line }) => {
    const scene = create(ImpactSceneSchema, {
      views: {
        '1': {
          placements: [{ element: { elementId: 10, viewId: 1, name: 'Source' } }],
          connectors: [{ id: 1, viewId: 1, sourceElementId: 10, targetElementId: 20, direction, label, tags: change ? ['change:' + change] : [] }],
        },
      },
    })
    const target = create(ScenePlacementSchema, { element: { elementId: 20, viewId: targetView, name: 'Target' } })
    if (targetView === 1) scene.views['1'].placements.push(target)
    else {
      scene.views[String(targetView)] = create(SceneViewContentSchema, { placements: [target] })
      scene.views['1'].connectorTargets['1'] = create(ImpactPlacementRefSchema, { viewId: targetView, elementId: 20 })
    }
    expect(impactSceneMermaid(scene)).toContain('\n  ' + line + '\n')
  })
})
