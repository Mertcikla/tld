/**
 * @tldiagram/core-ui the open-source core of the tlDiagram editor.
 *
 * This barrel file is the public API surface of the library.
 * It re-exports pages, components, hooks, contexts, types, and utilities
 * so that host applications (SaaS, CLI viewer, VS Code extension) can
 * compose them with their own routing, API clients, and pro features.
 */

// ─── Pages ───────────────────────────────────────────────────────────────────
export { default as ViewEditor } from './pages/ViewEditor'
export { default as ViewsPage } from './pages/Views'
export { default as ViewsGrid } from './pages/ViewsGrid'
export { default as Dependencies } from './pages/Dependencies'
export { default as InfiniteZoom, SharedInfiniteZoom } from './pages/InfiniteZoom'
export { default as Settings } from './pages/Settings'
export { default as AppearanceSettings } from './pages/AppearanceSettings'

// ─── UI Components ──────────────────────────────────────────────────────────
export { default as ElementNode } from './components/ElementNode'
export { default as ViewGridNode, type ViewGridNodeData } from './components/ViewGridNode'
export { default as ElementPanel, type ElementPanelProps } from './components/ElementPanel'
export { default as ConnectorPanel, type ConnectorPanelProps } from './components/ConnectorPanel'
export { default as ElementLibrary } from './components/ElementLibrary'
export { default as TopMenuBar } from './components/TopMenuBar'
export { default as ViewFloatingMenu } from './components/ViewFloatingMenu'
export type { ViewFloatingMenuProps } from './components/ViewFloatingMenu'
export { default as ViewBezierConnector } from './components/ViewBezierConnector'
export { default as ViewHeaderButton } from './components/ViewHeaderButton'
export { default as ViewPanel } from './components/ViewPanel'
export { default as ViewDrawMenu } from './components/ViewDrawMenu'
export { default as FloatingEdge } from './components/FloatingEdge'
export { default as ContextBoundaryElement } from './components/ContextBoundaryElement'
export { default as ContextNeighborElement } from './components/ContextNeighborElement'
export { default as ContextStraightConnector } from './components/ContextStraightConnector'
export { default as ProxyConnectorEdge } from './components/ProxyConnectorEdge'
export { default as ProxyConnectorPanel } from './components/ProxyConnectorPanel'
export { default as DrawingCanvas } from './components/DrawingCanvas'
export type { DrawingCanvasHandle, DrawingPath } from './components/DrawingCanvas'
export { default as InlineElementAdder } from './components/InlineElementAdder'
export { default as ExportModal } from './components/ExportModal'
export type { ExportOptions } from './components/ExportModal'
export { default as ImportModal } from './components/ImportModal'
export { default as ConfirmDialog } from './components/ConfirmDialog'
export { default as SlidingPanel } from './components/SlidingPanel'
export { default as PanelHeader } from './components/PanelHeader'
export { KbdHint } from './components/PanelUI'
export { default as TagUpsert } from './components/TagUpsert'
export { default as GitSourceLinker } from './components/GitSourceLinker'
export { default as LocalSourceLinker } from './components/LocalSourceLinker'
export { default as CodePreviewPanel } from './components/CodePreviewPanel'
export { SafeBackground } from './components/SafeBackground'
export { ElementBody as NodeBody } from './components/NodeBody'
export { ElementContainer as NodeContainer } from './components/NodeContainer'
export type { ElementContainerProps } from './components/NodeContainer'
export { default as NodeHoverCard } from './components/NodeHoverCard'
export { default as NavBreadcrumb } from './components/NavBreadcrumb'
export { default as ScrollIndicatorWrapper } from './components/ScrollIndicatorWrapper'
export { default as LayoutSection } from './components/LayoutSection'
export { default as CrossBranchControls } from './components/CrossBranchControls'

// Icons
export * from './components/Icons'

// ViewExplorer
export { default as ViewExplorer } from './components/ViewExplorer'
export * from './components/ViewExplorer/utils'
export type { NavItem, TreeNode } from './components/ViewExplorer/types'
export { ViewNavigator } from './components/ViewExplorer/ViewNavigator'
export { ViewSearch } from './components/ViewExplorer/ViewSearch'
export { ViewTree } from './components/ViewExplorer/ViewTree'
export { TagManager } from './components/ViewExplorer/TagManager'

// ViewEditor sub-components
export { ViewEditorContext, useViewEditorContext } from './pages/ViewEditor/context'

// Onboarding components
export { default as ViewEditorOnboarding } from './components/ViewEditorOnboarding'
export { default as DependenciesOnboarding } from './components/DependenciesOnboarding'
export { default as ViewsGridOnboarding } from './components/ViewsGridOnboarding'
export { default as ExploreOnboarding } from './components/ExploreOnboarding'
export { default as ExplorePageOnboarding } from './components/ExplorePageOnboarding'
export { default as MiniZoomOnboarding } from './components/MiniZoomOnboarding'

// ViewEditor edge label layout
export * from './components/ViewEditorEdgeLabelLayout'

// ─── ZUI ─────────────────────────────────────────────────────────────────────
export * from './components/ZUI'

// ─── Theme & Styles ──────────────────────────────────────────────────────────
export { default as theme } from './theme'

// ─── Contexts ────────────────────────────────────────────────────────────────
export { ThemeProvider, useAccentColor, useTheme } from './context/ThemeContext'
export { HeaderProvider, useSetHeader, useHeader } from './components/HeaderContext'

// ─── Types ───────────────────────────────────────────────────────────────────
export * from './types'

// ─── Platform ────────────────────────────────────────────────────────────────
export type { PlatformFeatures, PlatformRouteContext } from './platform/types'
export { platform as localPlatform } from './platform/local'
export { PlatformProvider } from './platform/PlatformContext'
export { usePlatform } from './platform/context'

// ─── API Contract ────────────────────────────────────────────────────────────
// The library ships with a reference stub client (offline/local mode).
// Host applications can provide their own implementation via the same interface.
export { api } from './api/client'
export type { DependenciesResponse } from './api/client'

// ─── Extension Slots ─────────────────────────────────────────────────────────
export type {
  TopMenuBarSlots,
  ElementPanelSlots,
  ConnectorPanelSlots,
  ViewFloatingMenuSlots,
  ViewEditorSlots,
  CoreUISlots,
} from './slots'

// ─── Types ───────────────────────────────────────────────────────────────────
export * from './types'

// ─── Hooks ───────────────────────────────────────────────────────────────────
export { useSafeFitView } from './hooks/useSafeFitView'

// ViewEditor hooks
export { useViewData } from './pages/ViewEditor/hooks/useViewData'
export { useDrawingEngine } from './pages/ViewEditor/hooks/useDrawingEngine'
export { useCanvasInteractions } from './pages/ViewEditor/hooks/useCanvasInteractions'
export { useViewContextNeighbours } from './pages/ViewEditor/hooks/useViewContextNeighbours'

// ViewEditor sub-components
export { EmptyCanvasState } from './pages/ViewEditor/components/EmptyCanvasState'
export { EditorOverlays } from './pages/ViewEditor/components/EditorOverlays'
export { ConnectorContextMenu, CanvasContextMenu } from './pages/ViewEditor/components/EditorMenus'

// ViewEditor utils
export * from './pages/ViewEditor/utils'

// ─── CrossBranch ─────────────────────────────────────────────────────────────
export * from './crossBranch/graph'
export * from './crossBranch/resolve'
export * from './crossBranch/settings'
export * from './crossBranch/store'
export * from './crossBranch/types'

// ─── Demo ────────────────────────────────────────────────────────────────────
export * from './demo/viewEditor'

// ─── Utilities ───────────────────────────────────────────────────────────────
export * from './utils/edgeDistribution'
export * from './utils/githubApi'
export * from './utils/githubCache'
export * from './utils/ids'
export * from './utils/technologyCatalog'
export { toast, ToastContainer } from './utils/toast'
export * from './utils/url'
export * from './utils/treesitter'

// ─── Constants ───────────────────────────────────────────────────────────────
export * from './constants/colors'
export * from './constants/diagramColors'

// ─── Config ──────────────────────────────────────────────────────────────────
export {
  appBase,
  routerBasename,
  isNativeApp,
  apiBase,
  apiUrl,
} from './config/runtime'

// ─── Lib ─────────────────────────────────────────────────────────────────────
export { vscodeBridge } from './lib/vscodeBridge'

// ─── Importer ────────────────────────────────────────────────────────────────
export { parseMermaid as parseMermaidToElements, type ParsedImport } from './pkg/importer/mermaid'

// ─── VS Code Messages ────────────────────────────────────────────────────────
export * from './types/vscode-messages'
