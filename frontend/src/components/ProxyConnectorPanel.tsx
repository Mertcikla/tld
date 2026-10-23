import { Box, Button, HStack, Text, VStack, Flex, IconButton } from '@chakra-ui/react'
import { useNavigate } from 'react-router-dom'
import type { ProxyConnectorDetails, ProxyConnectorLeaf, ProxyEndpoint, WorkspaceGraphSnapshot } from '../crossBranch/types'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import { EditorIcon, TrashIcon } from './Icons'
import { ConnectorSummaryCard, UnderlyingConnectors } from './ConnectorDetails'
import { canvasPopoverNameButtonStyles } from '../styles/surfaces'
import { useViewEditorContext } from '../pages/ViewEditor/context'
import type { Connector } from '../types'

interface Props {
  isOpen: boolean
  onClose: () => void
  details: ProxyConnectorDetails | null
  snapshot: WorkspaceGraphSnapshot | null
  hasBackdrop?: boolean
  onEdit?: (connector: Connector) => void
  onDelete?: (connectorId: number, ownerViewId: number) => void
}

type ConnectorNavTarget = {
  viewId: number
  viewName: string
}

function buildNavigationTarget(viewId: number, fallbackName: string | null | undefined, snapshot: WorkspaceGraphSnapshot): ConnectorNavTarget {
  return {
    viewId,
    viewName: snapshot.viewById[viewId]?.name ?? fallbackName ?? `View ${viewId}`,
  }
}

function resolveOffViewEndpointTarget(
  endpoint: ProxyEndpoint,
  snapshot: WorkspaceGraphSnapshot,
  currentViewId: number | null,
): ConnectorNavTarget | null {
  if (!endpoint.externalToView || currentViewId == null) return null

  for (const ownerElementId of endpoint.contextPathElementIds ?? []) {
    const childViewId = snapshot.childViewIdByOwnerElementId[ownerElementId]
    if (childViewId != null && childViewId !== currentViewId) {
      return buildNavigationTarget(childViewId, snapshot.viewById[childViewId]?.name ?? null, snapshot)
    }
  }

  const anchorChildViewId = snapshot.childViewIdByOwnerElementId[endpoint.anchorElementId]
  if (anchorChildViewId != null && anchorChildViewId !== currentViewId) {
    return buildNavigationTarget(anchorChildViewId, snapshot.viewById[anchorChildViewId]?.name ?? null, snapshot)
  }

  if (endpoint.anchorViewId != null && endpoint.anchorViewId !== currentViewId) {
    return buildNavigationTarget(endpoint.anchorViewId, endpoint.anchorViewName, snapshot)
  }

  if (endpoint.placementViewId != null && endpoint.placementViewId !== currentViewId) {
    return buildNavigationTarget(endpoint.placementViewId, endpoint.placementViewName, snapshot)
  }

  return null
}

function resolveLeafNavigationTarget(
  leaf: ProxyConnectorLeaf,
  snapshot: WorkspaceGraphSnapshot | null,
  currentViewId: number | null,
): ConnectorNavTarget | null {
  if (!snapshot) return null
  return resolveOffViewEndpointTarget(leaf.source, snapshot, currentViewId)
    ?? resolveOffViewEndpointTarget(leaf.target, snapshot, currentViewId)
}

export default function ProxyConnectorPanel({
  isOpen,
  onClose,
  details,
  snapshot,
  hasBackdrop = true,
  onEdit,
  onDelete,
}: Props) {
  const navigate = useNavigate()
  const { canEdit, viewId } = useViewEditorContext()

  return (
    <SlidingPanel
      isOpen={isOpen}
      onClose={onClose}
      panelKey="proxy-connector-panel"
      data-testid="proxy-connector-panel"
      width={{ base: 'calc(100vw - 24px)', md: '300px' }}
      hasBackdrop={hasBackdrop}
      zIndex={950}
    >
      <PanelHeader title="Connectors" onClose={onClose} />

      <Box flex={1} overflowY="auto" py={0}>
        {details ? (
          <VStack align="stretch" spacing={2.5} px={3.5} py={2.5}>
            <ConnectorSummaryCard details={details} />
            <UnderlyingConnectors
              details={details}
              maxHeight="none"
              onNavigate={onClose}
              renderActions={(leaf) => {
                const navigationTarget = resolveLeafNavigationTarget(leaf, snapshot, viewId)
                const hasEditActions = canEdit && (onEdit || onDelete)
                if (!navigationTarget && !hasEditActions) return null
                return (
                  <HStack justify="space-between" spacing={1} flexWrap="wrap">
                    {navigationTarget ? (
                      <Button
                        size="xs"
                        variant="outline"
                        sx={canvasPopoverNameButtonStyles}
                        leftIcon={<EditorIcon size={10} />}
                        onClick={(event) => {
                          event.stopPropagation()
                          navigate(`/views/${navigationTarget.viewId}`)
                          onClose()
                        }}
                        title={`Open ${navigationTarget.viewName}`}
                      >
                        <Text as="span" isTruncated>Open {navigationTarget.viewName}</Text>
                      </Button>
                    ) : <Box />}
                    {canEdit && (
                      <HStack spacing={0.5} ml="auto">
                        {onEdit && (
                          <IconButton
                            aria-label="Edit connector"
                            icon={<EditorIcon size={13} />}
                            size="xs"
                            variant="ghost"
                            color="gray.400"
                            _hover={{ bg: 'whiteAlpha.100', color: 'var(--accent)' }}
                            onClick={(event) => {
                              event.stopPropagation()
                              onEdit(leaf.connector)
                            }}
                          />
                        )}
                        {onDelete && (
                          <IconButton
                            aria-label="Delete connector"
                            icon={<TrashIcon size={13} />}
                            size="xs"
                            variant="ghost"
                            color="red.400"
                            _hover={{ bg: 'red.900', color: 'red.100' }}
                            onClick={(event) => {
                              event.stopPropagation()
                              onDelete(leaf.connector.id, leaf.ownerViewId)
                            }}
                          />
                        )}
                      </HStack>
                    )}
                  </HStack>
                )
              }}
            />
          </VStack>
        ) : (
          <Flex h="full" align="center" justify="center" direction="column" gap={3}>
            <Text color="gray.500" fontSize="sm">Select a connector to inspect it.</Text>
          </Flex>
        )}
      </Box>
    </SlidingPanel>
  )
}
