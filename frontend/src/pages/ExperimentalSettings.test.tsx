import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ExperimentalProvider } from '../context/ExperimentalContext'
import ExperimentalSettings from './ExperimentalSettings'

vi.mock('../components/watch/WatchControlPanel', () => ({
  default: () => React.createElement('WatchControlPanelStub'),
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const passthrough = (name: string) => {
    const Component = ({ children, ...rest }: { children?: React.ReactNode }) =>
      ReactModule.createElement(name, rest, children)
    Component.displayName = name
    return Component
  }
  return {
    Box: passthrough('Box'),
    Checkbox: passthrough('Checkbox'),
    Divider: passthrough('Divider'),
    FormLabel: passthrough('FormLabel'),
    HStack: passthrough('HStack'),
    Link: passthrough('Link'),
    Text: passthrough('Text'),
    VStack: passthrough('VStack'),
  }
})

function installWatchEnabledStub() {
  const values = new Map<string, string>([['tld:experimental', JSON.stringify({ watchEnabled: true })]])
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
      clear: () => values.clear(),
    },
  })
}

function renderSettings(surface: 'pane' | 'page'): ReactTestRenderer {
  let renderer!: ReactTestRenderer
  act(() => {
    renderer = create(
      <ExperimentalProvider>
        <ExperimentalSettings surface={surface} />
      </ExperimentalProvider>,
    )
  })
  return renderer
}

describe('ExperimentalSettings surfaces', () => {
  beforeEach(() => {
    installWatchEnabledStub()
  })

  it('keeps the watch toggle but omits the watch panel on the pane surface', () => {
    const renderer = renderSettings('pane')
    expect(renderer.root.findAll((node) => (node.type as unknown as string) === 'WatchControlPanelStub')).toHaveLength(0)
    expect(renderer.root.findAll((node) => (node.type as unknown as string) === 'Checkbox')).toHaveLength(1)
  })

  it('renders the watch panel on the page surface', () => {
    const renderer = renderSettings('page')
    expect(renderer.root.findAll((node) => (node.type as unknown as string) === 'WatchControlPanelStub')).toHaveLength(1)
  })
})
