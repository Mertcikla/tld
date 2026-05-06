import { ChakraProvider } from '@chakra-ui/react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { FixtureSnapshotView, type FixtureSnapshotResponse } from './FixtureReview'

describe('FixtureSnapshotView', () => {
  it('renders golden fixture sections', () => {
    const data: FixtureSnapshotResponse = {
      manifest: {
        name: 'basic_route',
        status: 'approved',
        language: 'go',
        domain: 'http',
        framework: 'nethttp',
        type: 'basic_route',
        review_status: 'reviewed',
        accuracy: 'accurate',
      },
      snapshot: {
        name: 'basic_route',
        counts: { elements: 1, facts: 1, connectors: 1, views: 1, filter_decisions: 1 },
        elements: [{ owner_type: 'symbol', owner_key: 'go:main.go:function:Main', name: 'Main', kind: 'function', tags: ['entrypoint'] }],
        facts: [{ type: 'http.route', enricher: 'go.nethttp', stable_key: 'route:/healthz', file_path: 'main.go', name: 'GET /healthz' }],
        views: [{ owner_type: 'repository', owner_key: 'repository', name: 'basic_route', level: 0 }],
        connectors: [{ owner_type: 'reference', owner_key: 'ref:1', source: 'symbol:a', target: 'symbol:b', view: 'repository:repository' }],
        filter_decisions: [{ owner_type: 'symbol', owner_key: 'go:main.go:function:Main', decision: 'visible', signals: ['route'] }],
      },
    }

    const html = renderToStaticMarkup(
      <ChakraProvider>
        <FixtureSnapshotView fixture="go/http/nethttp/basic_route" data={data} />
      </ChakraProvider>,
    )

    expect(html).toContain('basic_route')
    expect(html).toContain('Elements')
    expect(html).toContain('http.route')
    expect(html).toContain('Filter Decisions')
    expect(html).toContain('visible')
  })
})
