import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Box, Flex, HStack, IconButton, Tooltip } from '@chakra-ui/react'
import { EditIcon } from '@chakra-ui/icons'
import { ZoomInIcon } from '../components/Icons'
import { api } from '../api/client'
import { elementGroupTagForLayer } from '../utils/elementGroups'
import { isGroundingIgnoreTag } from '../utils/groundingTags'
import ConnectorPanel from '../components/ConnectorPanel'
import ElementPanel from '../components/ElementPanel'
import ViewPanel from '../components/ViewPanel'
import InspectDrawer from '../components/InventoryInspector'
import { ViewEditorContext } from './ViewEditor/context'
import type { Connector, LibraryElement, ViewTreeNode } from '../types'
import {
  buildInventoryRows,
  dependencyConnectorToConnector,
  flattenInventoryViews,
  type InventoryRow,
  type InventoryObjectType,
} from './inventoryData'
import { useInventoryTable } from './useInventoryTable'
import InventoryList from './InventoryList'
import InventoryCreateDialog from './InventoryCreateDialog'

export default function Inventory() {
  const navigate = useNavigate()
  const refreshVersion = useRef(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [mutationError, setMutationError] = useState<string | null>(null)
  const [elements, setElements] = useState<LibraryElement[]>([])
  const [views, setViews] = useState<ViewTreeNode[]>([])
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [countsByView, setCountsByView] = useState<
    Record<number, { placements: number; connectors: number }>
  >({})
  const [placementByViewElement, setPlacementByViewElement] = useState<
    Record<string, { x: number; y: number }>
  >({})
  const [availableTags, setAvailableTags] = useState<string[]>([])
  const [editing, setEditing] = useState<InventoryRow | null>(null)
  const [creating, setCreating] = useState<InventoryObjectType | null>(null)
  const [bulkLoading, setBulkLoading] = useState(false)
  const [tagColorMap, setTagColorMap] = useState<Record<string, { color: string }>>({})
  const [groupTagMeta, setGroupTagMeta] = useState<
    Record<string, { label: string; color: string; layerId: number; viewId: number }>
  >({})
  const refresh = useCallback(async () => {
    const version = ++refreshVersion.current
    setLoading(true)
    setLoadError(null)
    try {
      const [allElements, gridData, dependencies, fetchedTagColors] = await Promise.all([
        api.elements.list({ limit: 0 }),
        api.workspace.views.gridData(),
        api.dependencies.list(),
        api.workspace.orgs.tagColors.list().catch(() => ({})),
      ])
      const flatViews = flattenInventoryViews(gridData.views)
      const layersPerView = await Promise.all(
        flatViews.map((view) => api.workspace.views.layers.list(view.id).catch(() => [])),
      )
      const allLayers = layersPerView.flat()
      const nextCounts = Object.fromEntries(
        Object.entries(gridData.content).map(([viewId, content]) => [
          Number(viewId),
          { placements: content.placements.length, connectors: content.connectors.length },
        ]),
      )
      const nextPlacementLookup: Record<string, { x: number; y: number }> = {}
      Object.entries(gridData.content).forEach(([viewId, content]) => {
        content.placements.forEach((placement) => {
          nextPlacementLookup[`${viewId}:${placement.element_id}`] = {
            x: placement.position_x,
            y: placement.position_y,
          }
        })
      })
      const nextConnectors = dependencies.connectors.map(dependencyConnectorToConnector)
      const nextGroupTagMeta: Record<
        string,
        { label: string; color: string; layerId: number; viewId: number }
      > = {}
      allLayers.forEach((layer) => {
        const groupTag = elementGroupTagForLayer(layer)
        if (groupTag) {
          nextGroupTagMeta[groupTag] = {
            label: `group:${layer.name}`,
            color: layer.color || '#A0AEC0',
            layerId: layer.id,
            viewId: layer.diagram_id,
          }
        }
      })
      const tagSet = new Set<string>(Object.keys(fetchedTagColors))
      allElements.forEach((element) =>
        element.tags.forEach((tag) => {
          if (!isGroundingIgnoreTag(tag)) tagSet.add(tag)
        }),
      )
      flatViews.forEach((view) =>
        (view.tags ?? []).forEach((tag) => {
          if (!isGroundingIgnoreTag(tag)) tagSet.add(tag)
        }),
      )
      nextConnectors.forEach((connector) =>
        (connector.tags ?? []).forEach((tag) => {
          if (!isGroundingIgnoreTag(tag)) tagSet.add(tag)
        }),
      )
      allLayers.forEach((layer) =>
        layer.tags.forEach((tag) => {
          if (!isGroundingIgnoreTag(tag)) tagSet.add(tag)
        }),
      )
      if (version !== refreshVersion.current) return
      setElements(allElements)
      setViews(flatViews)
      setConnectors(nextConnectors)
      setCountsByView(nextCounts)
      setPlacementByViewElement(nextPlacementLookup)
      setGroupTagMeta(nextGroupTagMeta)
      setAvailableTags(Array.from(tagSet).sort((a, b) => a.localeCompare(b)))
      setTagColorMap(fetchedTagColors as Record<string, { color: string }>)
    } catch (error) {
      if (version === refreshVersion.current)
        setLoadError(error instanceof Error ? error.message : 'Could not load inventory.')
    } finally {
      if (version === refreshVersion.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const rows = useMemo(
    () => buildInventoryRows(elements, views, connectors, countsByView),
    [elements, views, connectors, countsByView],
  )
  const inventory = useInventoryTable(rows, loading)
  const { selectedRow } = inventory
  const setParam = (key: string, value: string) => inventory.updateParams({ [key]: value }, false)

  const handleCreated = async (key: string) => {
    inventory.updateParams({ q: '', type: '', tags: '', kind: '', qualities: '', object: key }, false)
    await refresh()
  }

  const handleTag = async (tag: string, remove: boolean) => {
    await runMutation(inventory.selectedRows, async (row) => {
      const tags = remove
        ? row.filterTags.filter((existing) => existing !== tag)
        : Array.from(new Set([...row.filterTags, tag]))
      if (row.element) await api.workspace.elements.update(row.id, { tags })
      else if (row.view) await api.workspace.views.update(row.id, { name: row.name, tags })
      else if (row.connector) await api.workspace.connectors.update(row.connector.view_id, row.id, { tags })
    })
  }

  // Settle every operation before refreshing. Keep failures selected for a safe retry.
  const runMutation = async (targets: InventoryRow[], mutate: (row: InventoryRow) => Promise<void>) => {
    setBulkLoading(true)
    setMutationError(null)
    try {
      const results = await Promise.allSettled(targets.map(mutate))
      const failed = targets.filter((_row, index) => results[index].status === 'rejected')
      inventory.setRowSelection(Object.fromEntries(failed.map((row) => [row.key, true])))
      if (failed.length)
        setMutationError(
          `${targets.length - failed.length} of ${targets.length} objects updated. ${failed.length} failed and remain selected; retry the action.`,
        )
      await refresh()
      return failed.length === 0
    } finally {
      setBulkLoading(false)
    }
  }

  const handleDelete = async (targets: InventoryRow[]) => {
    const success = await runMutation(targets, async (row) => {
      if (row.objectType === 'element') await api.workspace.elements.delete('', row.id)
      else if (row.objectType === 'view') await api.workspace.views.delete('', row.id)
      else await api.workspace.connectors.delete('', row.id)
    })
    if (success) setParam('object', '')
    return success
  }

  const handleDeleteTag = async (tag: string) => {
    const meta = groupTagMeta[tag]
    if (meta) {
      const results = await Promise.allSettled(
        elements
          .filter((element) => element.tags.includes(tag))
          .map((element) =>
            api.workspace.elements.update(element.id, {
              tags: element.tags.filter((existing) => existing !== tag),
            }),
          ),
      )
      if (results.some((result) => result.status === 'rejected')) {
        await refresh()
        throw new Error('Could not ungroup every element. Please retry.')
      }
      await api.workspace.views.layers.delete(meta.viewId, meta.layerId)
    }
    await api.workspace.orgs.tagColors.delete(tag)
    await refresh()
  }

  const navigationUrls = useMemo(() => {
    if (!selectedRow) return { exploreUrl: '', editorUrl: '' }

    if (selectedRow.objectType === 'view') {
      return {
        exploreUrl: `/views?view=explore&focus=${selectedRow.id}`,
        editorUrl: `/views/${selectedRow.id}`,
      }
    }

    if (selectedRow.objectType === 'element') {
      const placementKeys = Object.keys(placementByViewElement)
      const elementPlacementKey = placementKeys.find((k) => k.endsWith(`:${selectedRow.id}`))
      const viewId = elementPlacementKey ? Number(elementPlacementKey.split(':')[0]) : null
      const fallbackViewId = viewId || views[0]?.id || 1

      return {
        exploreUrl: `/views?view=explore&focus=${fallbackViewId}&element=${selectedRow.id}`,
        editorUrl: `/views/${fallbackViewId}?element=${selectedRow.id}`,
      }
    }

    if (selectedRow.objectType === 'connector' && selectedRow.connector) {
      const conn = selectedRow.connector
      return {
        exploreUrl: `/views?view=explore&focus=${conn.view_id}&element=${conn.source_element_id}`,
        editorUrl: `/views/${conn.view_id}`,
      }
    }

    return { exploreUrl: '', editorUrl: '' }
  }, [selectedRow, views, placementByViewElement])
  const editorContext = useMemo(
    () => ({
      viewId: editing?.connector?.view_id ?? selectedRow?.connector?.view_id ?? null,
      canEdit: true,
      isOwner: true,
      isFreePlan: false,
      snapToGrid: false,
      setSnapToGrid: () => undefined,
      autoLayoutMode: false,
      setAutoLayoutMode: () => undefined,
      selectedElement: editing?.element ?? selectedRow?.element ?? null,
      selectedConnector: editing?.connector ?? selectedRow?.connector ?? null,
    }),
    [editing, selectedRow],
  )

  return (
    <ViewEditorContext.Provider value={editorContext}>
      <Box
        data-testid="inventory-page"
        h="100%"
        bg="var(--bg-canvas)"
        display="flex"
        flexDir="column"
        overflow="hidden"
      >
        <Flex flex={1} minH={0} overflow="hidden">
          <Flex
            flex={1}
            minW={0}
            direction="column"
            overflowX="hidden"
            overflowY={{ base: 'auto', md: 'hidden' }}
          >
            <InventoryList
              state={inventory}
              rows={rows}
              loading={loading}
              error={mutationError ?? loadError}
              busy={bulkLoading}
              availableTags={availableTags}
              tagColorMap={tagColorMap}
              groupTagMeta={groupTagMeta}
              onRefresh={() => {
                setMutationError(null)
                void refresh()
              }}
              onEdit={(row) => {
                inventory.selectRow(row)
                if (window.matchMedia('(max-width: 1279px)').matches) setEditing(row)
              }}
              onCreate={setCreating}
              onDelete={handleDelete}
              onTag={(tag, remove) => {
                void handleTag(tag, remove)
              }}
              onDeleteTag={handleDeleteTag}
            />
            <Box flexShrink={0}>
              <InspectDrawer
                selectedRow={selectedRow}
                elements={elements}
                views={views}
                connectors={connectors}
                placementByViewElement={placementByViewElement}
                onSelectRow={(key) => {
                  const found = rows.find((r) => r.key === key)
                  if (found) inventory.selectRow(found)
                }}
              />
            </Box>
          </Flex>

          <Box
            w={{ base: '0', xl: '340px' }}
            display={{ base: 'none', xl: 'flex' }}
            flexDir="column"
            borderLeft="1px solid"
            borderColor="whiteAlpha.100"
            overflow="hidden"
            position="relative"
          >
            <Box flex={1} minH={0} overflow="auto" position="relative">
              {selectedRow?.objectType === 'view' && (
                <ViewPanel
                  isOpen={true}
                  onClose={() => {}}
                  view={selectedRow.view ?? null}
                  canEdit
                  onSave={() => refresh()}
                  availableTags={availableTags}
                  hasBackdrop={false}
                  isInline={true}
                  actions={
                    <HStack spacing={0.5}>
                      <Tooltip label="Explore" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Explore"
                          data-testid="inventory-action-explore"
                          icon={<ZoomInIcon size={15} strokeWidth={2.5} />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'rgba(var(--accent-rgb), 0.15)' }}
                          onClick={() => navigate(navigationUrls.exploreUrl)}
                        />
                      </Tooltip>
                      <Tooltip label="Open in Editor" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Open in Editor"
                          data-testid="inventory-action-editor"
                          icon={<EditIcon boxSize="14px" />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                          onClick={() => navigate(navigationUrls.editorUrl)}
                        />
                      </Tooltip>
                    </HStack>
                  }
                />
              )}
              {selectedRow?.objectType === 'element' && (
                <ElementPanel
                  isOpen={true}
                  onClose={() => {}}
                  element={selectedRow.element ?? null}
                  onSave={() => refresh()}
                  onDelete={() => {
                    setParam('object', '')
                    void refresh()
                  }}
                  onPermanentDelete={() => {
                    setParam('object', '')
                    void refresh()
                  }}
                  availableTags={availableTags}
                  orgId=""
                  hasBackdrop={false}
                  isInline={true}
                  autoSave
                  actions={
                    <HStack spacing={0.5}>
                      <Tooltip label="Explore" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Explore"
                          data-testid="inventory-action-explore"
                          icon={<ZoomInIcon size={15} strokeWidth={2.5} />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'rgba(var(--accent-rgb), 0.15)' }}
                          onClick={() => navigate(navigationUrls.exploreUrl)}
                        />
                      </Tooltip>
                      <Tooltip label="Open in Editor" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Open in Editor"
                          data-testid="inventory-action-editor"
                          icon={<EditIcon boxSize="14px" />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                          onClick={() => navigate(navigationUrls.editorUrl)}
                        />
                      </Tooltip>
                    </HStack>
                  }
                />
              )}
              {selectedRow?.objectType === 'connector' && (
                <ConnectorPanel
                  isOpen={true}
                  onClose={() => {}}
                  connector={selectedRow.connector ?? null}
                  orgId=""
                  onSave={() => refresh()}
                  onDelete={() => {
                    setParam('object', '')
                    void refresh()
                  }}
                  availableTags={availableTags}
                  hasBackdrop={false}
                  isInline={true}
                  autoSave
                  actions={
                    <HStack spacing={0.5}>
                      <Tooltip label="Explore" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Explore"
                          data-testid="inventory-action-explore"
                          icon={<ZoomInIcon size={15} strokeWidth={2.5} />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'rgba(var(--accent-rgb), 0.15)' }}
                          onClick={() => navigate(navigationUrls.exploreUrl)}
                        />
                      </Tooltip>
                      <Tooltip label="Open in Editor" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Open in Editor"
                          data-testid="inventory-action-editor"
                          icon={<EditIcon boxSize="14px" />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                          onClick={() => navigate(navigationUrls.editorUrl)}
                        />
                      </Tooltip>
                    </HStack>
                  }
                />
              )}
              {!selectedRow && (
                <Flex h="100%" align="center" justify="center" color="gray.600" fontSize="sm">
                  No object selected.
                </Flex>
              )}
            </Box>
          </Box>
        </Flex>

        <ElementPanel
          isOpen={creating === 'element' || editing?.objectType === 'element'}
          onClose={() => {
            setEditing(null)
            setCreating(null)
          }}
          element={editing?.element ?? null}
          onSave={(saved) => {
            setEditing(null)
            setCreating(null)
            void (creating === 'element' ? handleCreated(`element:${saved.id}`) : refresh())
          }}
          onDelete={() => {
            setEditing(null)
            void refresh()
          }}
          availableTags={availableTags}
          orgId=""
          hasBackdrop={false}
        />
        <ViewPanel
          isOpen={editing?.objectType === 'view'}
          onClose={() => {
            setEditing(null)
            setCreating(null)
          }}
          view={editing?.view ?? null}
          canEdit
          onSave={() => {
            setEditing(null)
            void refresh()
          }}
          availableTags={availableTags}
          hasBackdrop={false}
        />
        <ConnectorPanel
          isOpen={editing?.objectType === 'connector'}
          onClose={() => {
            setEditing(null)
            setCreating(null)
          }}
          connector={editing?.connector ?? null}
          orgId=""
          onSave={() => {
            setEditing(null)
            void refresh()
          }}
          onDelete={() => {
            setEditing(null)
            void refresh()
          }}
          availableTags={availableTags}
          hasBackdrop={false}
        />
        {creating && creating !== 'element' && (
          <InventoryCreateDialog
            type={creating}
            elements={elements}
            views={views}
            placementByViewElement={placementByViewElement}
            onClose={() => setCreating(null)}
            onCreated={handleCreated}
          />
        )}
      </Box>
    </ViewEditorContext.Provider>
  )
}
