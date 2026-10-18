import { readFileSync } from 'node:fs'
import { create } from '@bufbuild/protobuf'
import { ChangeKind, ImpactPlacementRefSchema, ImpactSceneSchema, ScenePlacementSchema, SceneViewContentSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/codeindex/v1/codeindex_pb.js'
import { describe, expect, it } from 'vitest'
import defaultTheme from '../../../internal/mermaid/theme.json'
import { CHANGE_RING_COLORS } from '../components/ZUI/changeOverlay'
import { EDGE_CHANGE_META } from '../components/ZUI/edgeChange'
import { ELEMENT_DEFAULT } from '../constants/colors'
import { impactSceneMermaid } from './impactSceneMermaid'

const cases = JSON.parse(readFileSync(new URL('../../../internal/mermaid/fixtures/connector-directions.json', import.meta.url), 'utf8')) as {
  direction: string; change: string; label: string; targetView: number; line: string; noThemeLine: string
}[]

describe('comparison Mermaid connector directions', () => {
  it.each(cases)('$direction/$change preserves endpoints and arrows', ({ direction, change, label, targetView, line, noThemeLine }) => {
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
    const unstyled = impactSceneMermaid(scene, { noTheme: true })
    expect(unstyled).toContain('\n  ' + noThemeLine + '\n')
    expect(unstyled).not.toContain('linkStyle')
  })
})

describe('comparison Mermaid theme frontmatter', () => {
  it('keeps the shared export palette aligned with canvas nodes and change rings', () => {
    for (const state of ['context', 'modified', 'added', 'removed'] as const) {
      expect(defaultTheme.nodes[state].fill).toBe(ELEMENT_DEFAULT)
      expect(defaultTheme.nodes[state].stroke).toBe(CHANGE_RING_COLORS[state === 'context' ? 'unchanged' : state])
      if (state !== 'context') expect(defaultTheme.edges[state].stroke).toBe(EDGE_CHANGE_META[state].color)
    }
  })
  it('prepends the embedded base theme only to themed exports', () => {
    const scene = create(ImpactSceneSchema)
    const output = impactSceneMermaid(scene)
    expect(output).toMatch(/^---\nconfig:\n {2}theme: 'base'\n {2}look: classic\n {2}htmlLabels: true\n/)
    expect(output).toContain("  themeCSS: '.edgeLabel .labelBkg { background: transparent !important; }'\n  themeVariables:")
    expect(output).toContain('\n---\nflowchart LR\n')
    for (const key of ['background', 'clusterBkg', 'edgeLabelBackground']) {
      expect(output).toContain(`    ${key}: "transparent"\n`)
    }
    expect(output).toContain('    darkMode: true\n')
    expect(output).toContain(`    primaryColor: "${ELEMENT_DEFAULT}"\n`)
    const unstyled = impactSceneMermaid(scene, { noTheme: true })
    expect(unstyled).toMatch(/^flowchart LR\n/)
    expect(unstyled).not.toContain('config:')
  })
})

describe('comparison Mermaid line count colors', () => {
  it('colors additions yellow and removals red, with plain no-theme fallback', () => {
    const scene = create(ImpactSceneSchema, { views: { '1': { placements: [{
      element: { elementId: 10, viewId: 1, name: 'Service' },
      overlay: { change: ChangeKind.MODIFIED, linesAdded: 3, linesRemoved: 2 },
    }] } } })
    expect(impactSceneMermaid(scene)).toContain("Service<br/><span style='color:#ecc94b!important'>+3</span> <span style='color:#fc8181!important'>−2</span>")
    const plain = impactSceneMermaid(scene, { noTheme: true })
    expect(plain).toContain('Service<br/>modified +3 −2')
    expect(plain).not.toContain('<span')
    scene.views['1'].placements[0].overlay!.linesAdded = 0
    expect(impactSceneMermaid(scene)).not.toContain('>+0</span>')
    scene.views['1'].placements[0].overlay!.linesRemoved = undefined
    expect(impactSceneMermaid(scene)).not.toContain('<span')
  })
})
