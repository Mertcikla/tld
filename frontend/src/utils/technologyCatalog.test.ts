import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { TechnologyCatalogItem } from '../types'

function stubBrowserGlobals() {
  vi.stubGlobal('window', {
    __TLD_VSCODE__: false,
    location: {
      protocol: 'http:',
      hostname: 'localhost',
      port: '5173',
    },
  })
}

function stubCatalog(items: TechnologyCatalogItem[]) {
  vi.stubGlobal('fetch', vi.fn(async () => ({
    ok: true,
    json: async () => items,
  } as Response)))
}

describe('technology catalog', () => {
  beforeEach(() => {
    vi.resetModules()
    stubBrowserGlobals()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('indexes catalog aliases for lookup and search', async () => {
    const custom: TechnologyCatalogItem = {
      iconUrl: '/icons/my-custom-icon.svg',
      name: 'My Custom Icon',
      nameShort: 'Custom',
      defaultSlug: 'my-custom-icon',
      aliases: ['custom-service'],
    }
    stubCatalog([custom])

    const catalog = await import('./technologyCatalog')

    await expect(catalog.getTechnologyCatalogItemBySlug('custom-service')).resolves.toBe(custom)
    await expect(catalog.searchTechnologyCatalog('custom-service')).resolves.toEqual([custom])
    expect(fetch).toHaveBeenCalledWith('/icons.json', { cache: 'no-store' })
  })

  it('lets catalog aliases override legacy slug aliases', async () => {
    const go: TechnologyCatalogItem = {
      iconUrl: '/icons/go.svg',
      name: 'Go',
      nameShort: 'Go',
      defaultSlug: 'go',
    }
    const custom: TechnologyCatalogItem = {
      iconUrl: '/icons/custom-go.svg',
      name: 'Custom Go',
      nameShort: 'Custom Go',
      defaultSlug: 'custom-go',
      aliases: ['golang'],
    }
    stubCatalog([go, custom])

    const catalog = await import('./technologyCatalog')

    await expect(catalog.getTechnologyCatalogItemBySlug('golang')).resolves.toBe(custom)
  })
})
