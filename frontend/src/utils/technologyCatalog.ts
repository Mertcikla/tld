import type { TechnologyCatalogItem } from '../types'
import { isNativeApp } from '../config/runtime'

interface SearchableCatalogItem {
  item: TechnologyCatalogItem
  haystack: string
}

type CatalogIndexTuple = [
  defaultSlug: string,
  name: string,
  nameShort?: string,
  provider?: string,
]

type CatalogMeta = Record<string, Partial<Pick<TechnologyCatalogItem, 'docsUrl' | 'description' | 'websiteUrl'>>>

interface TechnologyCatalogIndex {
  items: TechnologyCatalogItem[]
  searchable: SearchableCatalogItem[]
  bySlug: Map<string, TechnologyCatalogItem>
}

let indexPromise: Promise<TechnologyCatalogIndex> | null = null
let metaPromise: Promise<CatalogMeta> | null = null

export function resolveWithBase(urlOrPath: string): string {
  if (!urlOrPath) return urlOrPath
  if (urlOrPath.startsWith('http://') || urlOrPath.startsWith('https://') || urlOrPath.startsWith('data:')) {
    return urlOrPath
  }

  // When running inside the native mobile app (Capacitor), or inside an embedded webview
  // that serves content from localhost, avoid prefixing the app BASE_URL. Mobile builds and
  // the local webview often serve/resolve static assets from the web root (\"/\"), so
  // returning the path without adding import.meta.env.BASE_URL avoids requests to
  // unexpected URLs like \"https://localhost/app/icons/...\" which can 404.
  const runningOnLocalhost = typeof window !== 'undefined' && (() => {
    // file: is unequivocally a native / packaged environment (Capacitor)
    if (window.location.protocol === 'file:') return true

    // Only treat plain localhost/127.0.0.1 as the native webview when it's
    // served in a way that matches Capacitor's webview (typically https
    // without an explicit dev port). We must NOT treat the dev server
    // (e.g. http://localhost:5173) as native.
    const hostIsLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1'

    if (!hostIsLocal) return false

    // If served over HTTPS on localhost (Capacitor uses https://localhost) assume native/webview.
    if (window.location.protocol === 'https:') return true

    // If there's no explicit port (or it's the default HTTPS port), treat as native/webview.
    // Dev servers usually expose a non-default port (like 5173) - those should NOT be treated as native.
    const port = (window.location.port || '').trim()
    if (!port || port === '443') return true

    return false
  })()
  if (isNativeApp || runningOnLocalhost) {
    // Ensure the path is absolute so it resolves correctly from the app's web root.
    return urlOrPath.startsWith('/') ? urlOrPath : `/${urlOrPath}`
  }

  const base = import.meta.env.BASE_URL || '/'
  const normalizedBase = base.endsWith('/') ? base : `${base}/`
  if (urlOrPath.startsWith(normalizedBase) || urlOrPath === normalizedBase.slice(0, -1)) {
    return urlOrPath
  }
  const normalizedPath = urlOrPath.startsWith('/') ? urlOrPath.slice(1) : urlOrPath
  return `${normalizedBase}${normalizedPath}`
}

function normalizeText(value: string): string {
  return value.toLowerCase().trim()
}

function createHaystack(item: TechnologyCatalogItem): string {
  return normalizeText([
    item.name,
    item.nameShort,
    item.provider,
    item.defaultSlug,
  ].filter(Boolean).join(' '))
}

function itemFromIndexTuple(tuple: CatalogIndexTuple): TechnologyCatalogItem {
  const [defaultSlug, name, nameShort = '', provider = ''] = tuple
  return {
    defaultSlug,
    name,
    nameShort,
    provider,
    iconUrl: `/icons/${defaultSlug}.png`,
    docsUrl: '',
    description: '',
    websiteUrl: '',
  }
}

function normalizeCatalogItem(item: Partial<TechnologyCatalogItem>): TechnologyCatalogItem | null {
  const defaultSlug = String(item.defaultSlug || '').trim()
  const name = String(item.name || '').trim()
  if (!defaultSlug || !name) return null

  return {
    defaultSlug,
    name,
    nameShort: item.nameShort || '',
    provider: item.provider || '',
    iconUrl: item.iconUrl || `/icons/${defaultSlug}.png`,
    docsUrl: item.docsUrl || '',
    description: item.description || '',
    websiteUrl: item.websiteUrl || '',
  }
}

async function loadCatalogItems(): Promise<TechnologyCatalogItem[]> {
  const response = await fetch(resolveWithBase('icons.index.json'), { cache: 'force-cache' })
  if (!response.ok) {
    const fallback = await fetch(resolveWithBase('icons.json'), { cache: 'force-cache' })
    if (!fallback.ok) {
      throw new Error('Failed to load technology catalog')
    }
    const legacyData = await fallback.json()
    if (!Array.isArray(legacyData)) return []
    return legacyData
      .map((item) => normalizeCatalogItem(item))
      .filter((item): item is TechnologyCatalogItem => item !== null)
  }
  const data = await response.json()
  if (!Array.isArray(data)) return []

  return data
    .map((item) => {
      if (Array.isArray(item)) return itemFromIndexTuple(item as CatalogIndexTuple)
      return normalizeCatalogItem(item)
    })
    .filter((item): item is TechnologyCatalogItem => item !== null)
}

async function loadCatalogMeta(): Promise<CatalogMeta> {
  const response = await fetch(resolveWithBase('icons.meta.json'), { cache: 'force-cache' })
  if (!response.ok) return {}

  const data = await response.json()
  return data && typeof data === 'object' && !Array.isArray(data) ? data as CatalogMeta : {}
}

async function getCatalogMeta(): Promise<CatalogMeta> {
  if (!metaPromise) {
    metaPromise = loadCatalogMeta().catch((error) => {
      metaPromise = null
      throw error
    })
  }

  return metaPromise
}

export async function getTechnologyCatalogIndex(): Promise<TechnologyCatalogIndex> {
  if (!indexPromise) {
    indexPromise = loadCatalogItems().then((items) => {
      const bySlug = new Map<string, TechnologyCatalogItem>()
      const searchable: SearchableCatalogItem[] = []

      for (const item of items) {
        bySlug.set(item.defaultSlug, item)
        searchable.push({ item, haystack: createHaystack(item) })
      }

      return { items, searchable, bySlug }
    }).catch((error) => {
      indexPromise = null
      throw error
    })
  }

  return indexPromise
}

export async function searchTechnologyCatalog(query: string, maxResults = 12): Promise<TechnologyCatalogItem[]> {
  const normalizedQuery = query.trim()
  if (!normalizedQuery) return []

  const index = await getTechnologyCatalogIndex()

  try {
    const regex = new RegExp(normalizedQuery, 'i')
    const matches: TechnologyCatalogItem[] = []
    for (const entry of index.searchable) {
      if (regex.test(entry.haystack)) {
        matches.push(entry.item)
        if (matches.length >= maxResults) break
      }
    }
    return matches
  } catch {
    const needle = normalizeText(normalizedQuery)
    const matches: TechnologyCatalogItem[] = []
    for (const entry of index.searchable) {
      if (entry.haystack.includes(needle)) {
        matches.push(entry.item)
        if (matches.length >= maxResults) break
      }
    }
    return matches
  }
}

export async function getTechnologyCatalogItemBySlug(slug: string): Promise<TechnologyCatalogItem | null> {
  const cleanSlug = slug.trim()
  if (!cleanSlug) return null
  const index = await getTechnologyCatalogIndex()
  const item = index.bySlug.get(cleanSlug)
  if (!item) return null

  const meta = await getCatalogMeta().catch((): CatalogMeta => ({}))
  return {
    ...item,
    ...(meta[cleanSlug] ?? {}),
  }
}
