/**
 * localStorage-backed store for the demo mode.
 * Implements the subset of the `api` interface used by ViewEditor and its hooks.
 * Data is scoped under the `diag:demo:*` key namespace to avoid colliding
 * with a real logged-in session.
 */

import type {
  LibraryElement,
  PlacedElement,
  Connector,
  ViewTreeNode,
  ViewLayer,
  Tag,
  ElementPlacement,
  ExploreData,
} from '../types'
import {
  DEMO_ELEMENTS,
  DEMO_VIEWS,
  DEMO_PLACEMENTS,
  DEMO_CONNECTORS,
  DEMO_LAYERS,
} from './seed'

// ── Keys ──────────────────────────────────────────────────────────────────────

const K = {
  elements: 'diag:demo:elements',
  views: 'diag:demo:views',
  placements: (viewId: number) => `diag:demo:placements:${viewId}`,
  connectors: (viewId: number) => `diag:demo:connectors:${viewId}`,
  layers: (viewId: number) => `diag:demo:layers:${viewId}`,
  tagColors: 'diag:demo:tagColors',
  nextId: 'diag:demo:nextId',
} as const

// ── ID generation ──────────────────────────────────────────────────────────────

function nextId(): number {
  const current = Number(localStorage.getItem(K.nextId) ?? '9000')
  const next = current + 1
  localStorage.setItem(K.nextId, String(next))
  return next
}

// ── Generic persistence helpers ────────────────────────────────────────────────

function load<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    if (raw === null) return fallback
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}

function save(key: string, value: unknown): void {
  localStorage.setItem(key, JSON.stringify(value))
}

// ── Initialisation ─────────────────────────────────────────────────────────────
// Seed data is written once on first visit (when the store key is absent).

export function initDemoStore(): void {
  if (localStorage.getItem(K.elements) === null) {
    save(K.elements, DEMO_ELEMENTS)
  }
  if (localStorage.getItem(K.views) === null) {
    save(K.views, DEMO_VIEWS)
  }
  for (const [rawId, placements] of Object.entries(DEMO_PLACEMENTS)) {
    const id = Number(rawId)
    if (localStorage.getItem(K.placements(id)) === null) {
      save(K.placements(id), placements)
    }
  }
  for (const [rawId, connectors] of Object.entries(DEMO_CONNECTORS)) {
    const id = Number(rawId)
    if (localStorage.getItem(K.connectors(id)) === null) {
      save(K.connectors(id), connectors)
    }
  }
  for (const [rawId, layers] of Object.entries(DEMO_LAYERS)) {
    const id = Number(rawId)
    if (localStorage.getItem(K.layers(id)) === null) {
      save(K.layers(id), layers)
    }
  }
}

export function resetDemoStore(): void {
  localStorage.removeItem(K.elements)
  localStorage.removeItem(K.views)
  localStorage.removeItem(K.tagColors)
  localStorage.removeItem(K.nextId)
  localStorage.removeItem('diag:demo:accent-color')
  localStorage.removeItem('diag:demo:background-color')
  localStorage.removeItem('diag:demo:element-color')
  for (const viewId of getAllViewIds()) {
    localStorage.removeItem(K.placements(viewId))
    localStorage.removeItem(K.connectors(viewId))
    localStorage.removeItem(K.layers(viewId))
  }
  initDemoStore()
}

// ── Tree helpers ──────────────────────────────────────────────────────────────

function getAllViews(): ViewTreeNode[] {
  return load<ViewTreeNode[]>(K.views, DEMO_VIEWS)
}

function flattenTree(nodes: ViewTreeNode[]): ViewTreeNode[] {
  return nodes.flatMap((n) => [n, ...flattenTree(n.children ?? [])])
}

function getAllViewIds(): number[] {
  return flattenTree(getAllViews()).map((v) => v.id)
}

function findViewById(id: number): ViewTreeNode | null {
  return flattenTree(getAllViews()).find((v) => v.id === id) ?? null
}

function saveViews(roots: ViewTreeNode[]): void {
  save(K.views, roots)
}

function insertViewIntoTree(roots: ViewTreeNode[], newView: ViewTreeNode): ViewTreeNode[] {
  if (newView.parent_view_id === null) return [...roots, newView]
  return roots.map((n) => {
    if (n.id === newView.parent_view_id) return { ...n, children: [...(n.children ?? []), newView] }
    return { ...n, children: insertViewIntoTree(n.children ?? [], newView) }
  })
}

function deleteViewFromTree(roots: ViewTreeNode[], id: number): ViewTreeNode[] {
  return roots
    .filter((n) => n.id !== id)
    .map((n) => ({ ...n, children: deleteViewFromTree(n.children ?? [], id) }))
}

// ── api surface ───────────────────────────────────────────────────────────────

const NOW = () => new Date().toISOString()

export const demoApi = {
  explore: {
    load: async (): Promise<ExploreData> => {
      const tree = getAllViews()
      const flat = flattenTree(tree)
      const views: ExploreData['views'] = {}
      for (const v of flat) {
        views[v.id] = {
          placements: load<PlacedElement[]>(K.placements(v.id), []),
          connectors: load<Connector[]>(K.connectors(v.id), []),
        }
      }
      return { tree, views, navigations: [] }
    },
  },

  elements: {
    list: async (_params?: unknown): Promise<LibraryElement[]> => {
      return load<LibraryElement[]>(K.elements, DEMO_ELEMENTS)
    },

    get: async (id: number): Promise<LibraryElement> => {
      const elements = load<LibraryElement[]>(K.elements, DEMO_ELEMENTS)
      const el = elements.find((e) => e.id === id)
      if (!el) throw new Error(`Element ${id} not found`)
      return el
    },

    create: async (data: Partial<LibraryElement>): Promise<LibraryElement> => {
      const elements = load<LibraryElement[]>(K.elements, DEMO_ELEMENTS)
      const newEl: LibraryElement = {
        id: nextId(),
        name: data.name ?? 'New Element',
        kind: data.kind ?? null,
        description: data.description ?? null,
        technology: data.technology ?? null,
        url: data.url ?? null,
        logo_url: data.logo_url ?? null,
        technology_connectors: data.technology_connectors ?? [],
        tags: data.tags ?? [],
        repo: data.repo ?? null,
        branch: data.branch ?? null,
        file_path: data.file_path ?? null,
        language: data.language ?? null,
        created_at: NOW(),
        updated_at: NOW(),
        has_view: false,
        view_label: null,
      }
      save(K.elements, [...elements, newEl])
      return newEl
    },

    update: async (id: number, data: Partial<LibraryElement>): Promise<LibraryElement> => {
      const elements = load<LibraryElement[]>(K.elements, DEMO_ELEMENTS)
      const idx = elements.findIndex((e) => e.id === id)
      if (idx === -1) throw new Error(`Element ${id} not found`)
      const updated = { ...elements[idx], ...data, id, updated_at: NOW() }
      const next = [...elements]
      next[idx] = updated
      save(K.elements, next)
      // Patch all view placements that reference this element
      for (const viewId of getAllViewIds()) {
        const placements = load<PlacedElement[]>(K.placements(viewId), [])
        const changed = placements.map((p) =>
          p.element_id === id
            ? { ...p, name: updated.name, kind: updated.kind, description: updated.description, technology: updated.technology, tags: updated.tags }
            : p,
        )
        save(K.placements(viewId), changed)
      }
      return updated
    },

    delete: async (_orgId: string, id: number): Promise<void> => {
      const elements = load<LibraryElement[]>(K.elements, DEMO_ELEMENTS)
      save(K.elements, elements.filter((e) => e.id !== id))
      for (const viewId of getAllViewIds()) {
        const placements = load<PlacedElement[]>(K.placements(viewId), [])
        save(K.placements(viewId), placements.filter((p) => p.element_id !== id))
        const connectors = load<Connector[]>(K.connectors(viewId), [])
        save(K.connectors(viewId), connectors.filter((c) => c.source_element_id !== id && c.target_element_id !== id))
      }
    },

    placements: async (id: number): Promise<ElementPlacement[]> => {
      const result: ElementPlacement[] = []
      for (const viewId of getAllViewIds()) {
        const placements = load<PlacedElement[]>(K.placements(viewId), [])
        const match = placements.find((p) => p.element_id === id)
        if (match) result.push({ id: match.id, view_id: viewId, element_id: id, position_x: match.position_x, position_y: match.position_y })
      }
      return result
    },
  },

  workspace: {
    orgs: {
      tagColors: {
        list: async (): Promise<Record<string, Tag>> => {
          return load<Record<string, Tag>>(K.tagColors, {})
        },
        set: async (tag: string, color: string, description?: string): Promise<void> => {
          const tags = load<Record<string, Tag>>(K.tagColors, {})
          save(K.tagColors, { ...tags, [tag]: { name: tag, color, description: description ?? null } })
        },
      },
    },

    views: {
      list: async () => {
        return flattenTree(getAllViews()).map((v) => ({
          id: v.id,
          owner_element_id: v.owner_element_id ?? null,
          name: v.name,
          label: v.level_label,
          is_root: v.parent_view_id === null,
          created_at: v.created_at,
          updated_at: v.updated_at,
        }))
      },

      get: async (id: number): Promise<ViewTreeNode> => {
        const v = findViewById(id)
        if (!v) throw new Error(`View ${id} not found`)
        return v
      },

      content: async (id: number): Promise<{ placements: PlacedElement[]; connectors: Connector[] }> => {
        return {
          placements: load<PlacedElement[]>(K.placements(id), []),
          connectors: load<Connector[]>(K.connectors(id), []),
        }
      },

      tree: async (): Promise<ViewTreeNode[]> => {
        return getAllViews()
      },

      create: async (data: { name: string; label?: string; parent_view_id?: number | null }) => {
        const id = nextId()
        const now = NOW()
        const newView: ViewTreeNode = {
          id,
          name: data.name,
          description: null,
          level_label: data.label ?? null,
          level: 0,
          depth: 0,
          owner_element_id: data.parent_view_id ?? null,
          parent_view_id: data.parent_view_id ?? null,
          created_at: now,
          updated_at: now,
          children: [],
        }
        const roots = getAllViews()
        saveViews(insertViewIntoTree(roots, newView))
        save(K.placements(id), [])
        save(K.connectors(id), [])
        save(K.layers(id), [])

        // Mark the owning element as having a view
        if (data.parent_view_id != null) {
          const elements = load<LibraryElement[]>(K.elements, DEMO_ELEMENTS)
          const idx = elements.findIndex((e) => e.id === data.parent_view_id)
          if (idx !== -1) {
            const next = [...elements]
            next[idx] = { ...next[idx], has_view: true }
            save(K.elements, next)
          }
          // Patch all placements of that element to reflect has_view
          for (const viewId of getAllViewIds()) {
            const placements = load<PlacedElement[]>(K.placements(viewId), [])
            const changed = placements.map((p) =>
              p.element_id === data.parent_view_id ? { ...p, has_view: true } : p,
            )
            save(K.placements(viewId), changed)
          }
        }

        return {
          id,
          owner_element_id: data.parent_view_id ?? null,
          name: data.name,
          label: data.label ?? null,
          is_root: data.parent_view_id === null,
          created_at: now,
          updated_at: now,
        }
      },

      update: async (id: number, data: { name: string; label?: string }) => {
        const roots = getAllViews()
        const patchInTree = (nodes: ViewTreeNode[]): ViewTreeNode[] =>
          nodes.map((n) =>
            n.id === id
              ? { ...n, name: data.name, level_label: data.label ?? n.level_label, updated_at: NOW() }
              : { ...n, children: patchInTree(n.children ?? []) },
          )
        saveViews(patchInTree(roots))
        const v = findViewById(id)
        return {
          id,
          owner_element_id: v?.owner_element_id ?? null,
          name: data.name,
          label: data.label ?? null,
          is_root: v?.parent_view_id === null,
          created_at: v?.created_at ?? NOW(),
          updated_at: NOW(),
        }
      },

      delete: async (_orgId: string, id: number): Promise<void> => {
        const roots = getAllViews()
        saveViews(deleteViewFromTree(roots, id))
        localStorage.removeItem(K.placements(id))
        localStorage.removeItem(K.connectors(id))
        localStorage.removeItem(K.layers(id))
      },

      placements: {
        list: async (viewId: number): Promise<ElementPlacement[]> => {
          const placements = load<PlacedElement[]>(K.placements(viewId), [])
          return placements.map((p) => ({ id: p.id, view_id: viewId, element_id: p.element_id, position_x: p.position_x, position_y: p.position_y }))
        },

        add: async (viewId: number, elementId: number, x = 100, y = 100): Promise<ElementPlacement> => {
          const elements = load<LibraryElement[]>(K.elements, DEMO_ELEMENTS)
          const el = elements.find((e) => e.id === elementId)
          if (!el) throw new Error(`Element ${elementId} not found`)
          const placements = load<PlacedElement[]>(K.placements(viewId), [])
          const id = nextId()
          const newPlacement: PlacedElement = {
            id,
            view_id: viewId,
            element_id: elementId,
            position_x: x,
            position_y: y,
            name: el.name,
            kind: el.kind,
            description: el.description,
            technology: el.technology,
            url: el.url,
            logo_url: el.logo_url,
            technology_connectors: el.technology_connectors,
            tags: el.tags,
            repo: el.repo,
            branch: el.branch,
            file_path: el.file_path,
            language: el.language,
            has_view: el.has_view,
            view_label: el.view_label,
          }
          save(K.placements(viewId), [...placements, newPlacement])
          return { id, view_id: viewId, element_id: elementId, position_x: x, position_y: y }
        },

        updatePosition: async (viewId: number, elementId: number, x: number, y: number): Promise<void> => {
          const placements = load<PlacedElement[]>(K.placements(viewId), [])
          save(K.placements(viewId), placements.map((p) => p.element_id === elementId ? { ...p, position_x: x, position_y: y } : p))
        },

        remove: async (viewId: number, elementId: number): Promise<void> => {
          const placements = load<PlacedElement[]>(K.placements(viewId), [])
          save(K.placements(viewId), placements.filter((p) => p.element_id !== elementId))
        },
      },

      layers: {
        list: async (viewId: number): Promise<ViewLayer[]> => {
          return load<ViewLayer[]>(K.layers(viewId), [])
        },
        create: async (viewId: number, data: { name: string; tags: string[]; color?: string }): Promise<ViewLayer> => {
          const layers = load<ViewLayer[]>(K.layers(viewId), [])
          const id = nextId()
          const newLayer: ViewLayer = { id, diagram_id: viewId, name: data.name, tags: data.tags, color: data.color ?? '#888888', created_at: NOW(), updated_at: NOW() }
          save(K.layers(viewId), [...layers, newLayer])
          return newLayer
        },
        update: async (viewId: number, layerId: number, data: Partial<ViewLayer>): Promise<ViewLayer> => {
          const layers = load<ViewLayer[]>(K.layers(viewId), [])
          const idx = layers.findIndex((l) => l.id === layerId)
          if (idx === -1) throw new Error(`Layer ${layerId} not found`)
          const updated = { ...layers[idx], ...data, id: layerId, updated_at: NOW() }
          const next = [...layers]
          next[idx] = updated
          save(K.layers(viewId), next)
          return updated
        },
        delete: async (viewId: number, layerId: number): Promise<void> => {
          const layers = load<ViewLayer[]>(K.layers(viewId), [])
          save(K.layers(viewId), layers.filter((l) => l.id !== layerId))
        },
      },

      reactions: {
        list: async (_viewId: number) => [],
      },

      threads: {
        listForElement: async () => [],
        listForConnector: async () => [],
        createForElement: async () => { throw new Error('Demo: threads not supported') },
        createForConnector: async () => { throw new Error('Demo: threads not supported') },
        addComment: async () => { throw new Error('Demo: comments not supported') },
        resolve: async () => { /* no-op */ },
      },

      thumbnail: (_id: number) => Promise.resolve(null),
      rename: async (id: number, name: string) => demoApi.workspace.views.update(id, { name }),
      setLevel: async () => { /* no-op */ },
      reparent: async () => { throw new Error('Demo: reparent not supported') },
    },

    connectors: {
      list: async (viewId: number): Promise<Connector[]> => {
        return load<Connector[]>(K.connectors(viewId), [])
      },

      create: async (
        viewId: number,
        data: {
          source_element_id: number; target_element_id: number
          label?: string; description?: string; relationship?: string
          direction?: string; style?: string; url?: string
          source_handle?: string | null; target_handle?: string | null
        },
      ): Promise<Connector> => {
        const connectors = load<Connector[]>(K.connectors(viewId), [])
        const id = nextId()
        const now = NOW()
        const newConnector: Connector = {
          id,
          view_id: viewId,
          source_element_id: data.source_element_id,
          target_element_id: data.target_element_id,
          label: data.label ?? null,
          description: data.description ?? null,
          relationship: data.relationship ?? null,
          direction: data.direction ?? 'forward',
          style: data.style ?? 'bezier',
          url: data.url ?? null,
          source_handle: data.source_handle ?? null,
          target_handle: data.target_handle ?? null,
          created_at: now,
          updated_at: now,
        }
        save(K.connectors(viewId), [...connectors, newConnector])
        return newConnector
      },

      update: async (
        viewId: number,
        connectorId: number,
        data: Partial<Connector>,
      ): Promise<Connector> => {
        const connectors = load<Connector[]>(K.connectors(viewId), [])
        const idx = connectors.findIndex((c) => c.id === connectorId)
        if (idx === -1) throw new Error(`Connector ${connectorId} not found`)
        const updated = { ...connectors[idx], ...data, id: connectorId, updated_at: NOW() }
        const next = [...connectors]
        next[idx] = updated
        save(K.connectors(viewId), next)
        return updated
      },

      delete: async (_orgId: string, connectorId: number): Promise<void> => {
        for (const viewId of getAllViewIds()) {
          const connectors = load<Connector[]>(K.connectors(viewId), [])
          const filtered = connectors.filter((c) => c.id !== connectorId)
          if (filtered.length !== connectors.length) {
            save(K.connectors(viewId), filtered)
            break
          }
        }
      },
    },

    elements: {
      list: (params?: unknown) => demoApi.elements.list(params),
      get: (id: number) => demoApi.elements.get(id),
      create: (data: Partial<LibraryElement>) => demoApi.elements.create(data),
      update: (id: number, data: Partial<LibraryElement>) => demoApi.elements.update(id, data),
      delete: (orgId: string, id: number) => demoApi.elements.delete(orgId, id),
      placements: (id: number) => demoApi.elements.placements(id),
    },
  },
}
