import type { LibraryElement, PlacedElement, Connector, ViewTreeNode, ViewLayer } from '../types'

const NOW = new Date().toISOString()

export const DEMO_ELEMENTS: LibraryElement[] = [
  { id: 1, name: 'User', kind: 'person', description: 'End user of the system', technology: null, url: null, logo_url: "https://tldiagram.com/app/icons/azure-users.png", technology_connectors: [], tags: ['external'], has_view: false, view_label: null, created_at: NOW, updated_at: NOW },
  { id: 2, name: 'Web App', kind: 'service', description: 'React single-page application', technology: 'React', url: null, logo_url: "https://tldiagram.com/app/icons/react.png", technology_connectors: [], tags: ['frontend'], has_view: true, view_label: null, created_at: NOW, updated_at: NOW },
  { id: 3, name: 'API Gateway', kind: 'service', description: 'REST API gateway', technology: 'Golang', url: null, logo_url: "https://tldiagram.com/app/icons/golang.png", technology_connectors: [], tags: ['backend'], has_view: false, view_label: null, created_at: NOW, updated_at: NOW },
  { id: 4, name: 'Auth Service', kind: 'service', description: 'Handles authentication & sessions', technology: 'Go', url: null, logo_url: "https://tldiagram.com/app/icons/golang.png", technology_connectors: [], tags: ['backend'], has_view: false, view_label: null, created_at: NOW, updated_at: NOW },
  { id: 5, name: 'Frontend', kind: 'service', description: 'React frontend bundle', technology: 'React', url: null, logo_url: null, technology_connectors: [], tags: ['frontend'], has_view: false, view_label: null, created_at: NOW, updated_at: NOW },
  { id: 6, name: 'Backend', kind: 'service', description: 'Core business logic API', technology: 'Go', url: null, logo_url: "https://tldiagram.com/app/icons/golang.png", technology_connectors: [], tags: ['backend'], has_view: false, view_label: null, created_at: NOW, updated_at: NOW },
  { id: 7, name: 'PostgreSQL', kind: 'database', description: 'Primary relational database', technology: 'PostgreSQL', url: null, logo_url: null, technology_connectors: [], tags: ['data'], has_view: false, view_label: null, created_at: NOW, updated_at: NOW },
  { id: 8, name: 'Redis Cache', kind: 'database', description: 'In-memory cache and session store', technology: 'Redis', url: null, logo_url: null, technology_connectors: [], tags: ['data'], has_view: false, view_label: null, created_at: NOW, updated_at: NOW },
  { id: 9, name: 'CDN', kind: 'service', description: 'Content delivery network', technology: 'Cloudflare', url: null, logo_url: null, technology_connectors: [], tags: ['external', 'infrastructure'], has_view: false, view_label: null, created_at: NOW, updated_at: NOW },
]

export const DEMO_VIEWS: ViewTreeNode[] = [
  {
    id: 1, name: 'System Context', description: 'Top-level system context view', level_label: 'Context', level: 1, depth: 0,
    owner_element_id: null, parent_view_id: null, created_at: NOW, updated_at: NOW, children: [
      {
        id: 2, name: 'Web App – Containers', description: 'Container-level view of the Web App', level_label: 'Container', level: 2, depth: 1,
        owner_element_id: 2, parent_view_id: 1, created_at: NOW, updated_at: NOW, children: [],
      },
    ],
  },
]

type ViewPlacements = Record<number, PlacedElement[]>
type ViewConnectors = Record<number, Connector[]>

export const DEMO_PLACEMENTS: ViewPlacements = {
  1: [
    { id: 101, view_id: 1, element_id: 1, position_x: 80, position_y: 200, name: 'User', kind: 'person', description: 'End user of the system', technology: null, url: null, logo_url: "https://tldiagram.com/app/icons/azure-users.png", technology_connectors: [], tags: ['external'], has_view: false, view_label: null },
    { id: 102, view_id: 1, element_id: 2, position_x: 380, position_y: 200, name: 'Web App', kind: 'service', description: 'React single-page application', technology: 'React', url: null, logo_url: "https://tldiagram.com/app/icons/react.png", technology_connectors: [], tags: ['frontend'], has_view: true, view_label: null },
    { id: 103, view_id: 1, element_id: 3, position_x: 680, position_y: 200, name: 'API Gateway', kind: 'service', description: 'REST API gateway', technology: 'Go', url: null, logo_url: "https://tldiagram.com/app/icons/golang.png", technology_connectors: [], tags: ['backend'], has_view: false, view_label: null },
    { id: 104, view_id: 1, element_id: 4, position_x: 380, position_y: 400, name: 'Auth Service', kind: 'service', description: 'Handles authentication & sessions', technology: 'Auth0', url: null, logo_url: "https://tldiagram.com/app/icons/auth0.png", technology_connectors: [], tags: ['backend'], has_view: false, view_label: null },
    { id: 105, view_id: 1, element_id: 9, position_x: 380, position_y: 0, name: 'CDN', kind: 'service', description: 'Content delivery network', technology: 'Cloudflare', url: null, logo_url: "https://tldiagram.com/app/icons/cloudflare.png", technology_connectors: [], tags: ['external', 'infrastructure'], has_view: false, view_label: null },
    { id: 106, view_id: 1, element_id: 10, position_x: 680, position_y: 0, name: 'Stripe', kind: 'service', description: 'Payment', technology: 'Stripe', url: null, logo_url: "https://tldiagram.com/app/icons/stripe.png", technology_connectors: [], tags: ['external', 'billing'], has_view: false, view_label: null },
  ],
  2: [
    { id: 201, view_id: 2, element_id: 5, position_x: 80, position_y: 200, name: 'Frontend', kind: 'service', description: 'React frontend bundle', technology: 'React', url: null, logo_url: "https://tldiagram.com/app/icons/react.png", technology_connectors: [], tags: ['frontend'], has_view: false, view_label: null },
    { id: 202, view_id: 2, element_id: 6, position_x: 380, position_y: 200, name: 'Backend', kind: 'service', description: 'Core business logic API', technology: 'Go', url: null, logo_url: "https://tldiagram.com/app/icons/golang.png", technology_connectors: [], tags: ['backend'], has_view: false, view_label: null },
    { id: 203, view_id: 2, element_id: 7, position_x: 680, position_y: 200, name: 'PostgreSQL', kind: 'database', description: 'Primary relational database', technology: 'PostgreSQL', url: null, logo_url: "https://tldiagram.com/app/icons/postgresql.png", technology_connectors: [], tags: ['data'], has_view: false, view_label: null },
    { id: 204, view_id: 2, element_id: 8, position_x: 680, position_y: 400, name: 'Redis Cache', kind: 'database', description: 'In-memory cache and session store', technology: 'Redis', url: null, logo_url: "https://tldiagram.com/app/icons/redis.png", technology_connectors: [], tags: ['data'], has_view: false, view_label: null },
  ],
}

export const DEMO_CONNECTORS: ViewConnectors = {
  1: [
    { id: 1001, view_id: 1, source_element_id: 1, target_element_id: 2, label: 'Uses', description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: 'right', target_handle: 'left', created_at: NOW, updated_at: NOW },
    { id: 1002, view_id: 1, source_element_id: 2, target_element_id: 3, label: 'API calls', description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: 'right', target_handle: 'left', created_at: NOW, updated_at: NOW },
    { id: 1003, view_id: 1, source_element_id: 2, target_element_id: 4, label: 'Auth', description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: 'bottom', target_handle: 'top', created_at: NOW, updated_at: NOW },
    { id: 1004, view_id: 1, source_element_id: 2, target_element_id: 9, label: 'Serves via', description: null, relationship: null, direction: 'backward', style: 'bezier', url: null, source_handle: 'top', target_handle: 'bottom', created_at: NOW, updated_at: NOW },
    { id: 1005, view_id: 1, source_element_id: 3, target_element_id: 10, label: 'Billing', description: null, relationship: null, direction: 'both', style: 'bezier', url: null, source_handle: 'top', target_handle: 'bottom', created_at: NOW, updated_at: NOW },
  ],
  2: [
    { id: 2001, view_id: 2, source_element_id: 5, target_element_id: 6, label: 'HTTP/JSON', description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: 'right', target_handle: 'left', created_at: NOW, updated_at: NOW },
    { id: 2002, view_id: 2, source_element_id: 6, target_element_id: 7, label: 'Reads / Writes', description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: 'right', target_handle: 'left', created_at: NOW, updated_at: NOW },
    { id: 2003, view_id: 2, source_element_id: 6, target_element_id: 8, label: 'Cache', description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: 'bottom', target_handle: 'left', created_at: NOW, updated_at: NOW },
  ],
}

export const DEMO_LAYERS: Record<number, ViewLayer[]> = {
  1: [],
  2: [],
}
