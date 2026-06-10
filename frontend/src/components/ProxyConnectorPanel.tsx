import { Box, Button, HStack, Text, VStack, Divider, Flex, IconButton } from '@chakra-ui/react'
import { useNavigate } from 'react-router-dom'
import type { ProxyConnectorDetails, ProxyConnectorLeaf, ProxyEndpoint, WorkspaceGraphSnapshot } from '../crossBranch/types'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import { NavigationIcon, TrashIcon, DrawIcon } from './Icons'
import { useViewEditorContext } from '../pages/ViewEditor/context'
import type { Connector } from '../types'
import { truncate } from '../utils/string'

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

function ConnectorArrowIcon({ direction }: { direction: string }) {
  const strokeColor = 'var(--accent)'
  switch (direction) {
    case 'backward':
      return (
        <svg width="32" height="12" viewBox="0 0 32 12" fill="none" style={{ display: 'inline-block', verticalAlign: 'middle' }}>
          <line x1="8" y1="6" x2="28" y2="6" stroke={strokeColor} strokeWidth="2" />
          <polygon points="9,2.5 2.5,6 9,9.5" fill={strokeColor} />
          <circle cx="28" cy="6" r="2.5" fill={strokeColor} />
        </svg>
      )
    case 'both':
      return (
        <svg width="32" height="12" viewBox="0 0 32 12" fill="none" style={{ display: 'inline-block', verticalAlign: 'middle' }}>
          <line x1="8" y1="6" x2="24" y2="6" stroke={strokeColor} strokeWidth="2" />
          <polygon points="9,2.5 2.5,6 9,9.5" fill={strokeColor} />
          <polygon points="23,2.5 29.5,6 23,9.5" fill={strokeColor} />
        </svg>
      )
    case 'none':
      return (
        <svg width="32" height="12" viewBox="0 0 32 12" fill="none" style={{ display: 'inline-block', verticalAlign: 'middle' }}>
          <line x1="4" y1="6" x2="28" y2="6" stroke={strokeColor} strokeWidth="2" />
          <circle cx="4" cy="6" r="2.5" fill={strokeColor} />
          <circle cx="28" cy="6" r="2.5" fill={strokeColor} />
        </svg>
      )
    case 'forward':
    default:
      return (
        <svg width="32" height="12" viewBox="0 0 32 12" fill="none" style={{ display: 'inline-block', verticalAlign: 'middle' }}>
          <line x1="4" y1="6" x2="24" y2="6" stroke={strokeColor} strokeWidth="2" />
          <circle cx="4" cy="6" r="2.5" fill={strokeColor} />
          <polygon points="23,2.5 29.5,6 23,9.5" fill={strokeColor} />
        </svg>
      )
  }
}

function summarizedDirection(details: ProxyConnectorDetails) {
  const directions = new Set(details.connectors.map((leaf) => leaf.connector.direction))
  if (directions.has('both')) return 'both'
  if (directions.has('forward') && directions.has('backward')) return 'both'
  if (directions.has('forward')) return 'forward'
  if (directions.has('backward')) return 'backward'
  return 'none'
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
      width={{ base: 'calc(100vw - 24px)', md: '300px' }}
      hasBackdrop={hasBackdrop}
      zIndex={950}
    >
      <PanelHeader title="Connectors" onClose={onClose} />

      <Box flex={1} overflowY="auto" px={4} py={3}>
        {details ? (
          <VStack align="stretch" spacing={5}>
            {/* Summary header */}
            <Box>
              <HStack spacing={1.5} mb={1} minWidth={0} overflow="hidden">
                <Text
                  color="white"
                  fontSize="sm"
                  fontWeight="semibold"
                  letterSpacing="-0.01em"
                  isTruncated
                  flex={1}
                  minWidth={0}
                  textAlign="right"
                >
                  {details.sourceAnchorName}
                </Text>
                <Flex align="center" justify="center" flexShrink={0} px={1}>
                  <ConnectorArrowIcon direction={summarizedDirection(details)} />
                </Flex>
                <Text
                  color="white"
                  fontSize="sm"
                  fontWeight="semibold"
                  letterSpacing="-0.01em"
                  isTruncated
                  flex={1}
                  minWidth={0}
                >
                  {details.targetAnchorName}
                </Text>
              </HStack>
              <Text color="blue.300" fontSize="xs" fontWeight="600">
                {details.label}
              </Text>
            </Box>

            <Divider borderColor="whiteAlpha.100" />

            <VStack align="stretch" spacing={3}>
              <Text
                color="gray.500"
                fontSize="10px"
                fontWeight="800"
                letterSpacing="0.12em"
                textTransform="uppercase"
              >
                Underlying Connectors
              </Text>

              <VStack align="stretch" spacing={2}>
                {details.connectors.map((leaf, idx) => {
                  const navigationTarget = resolveLeafNavigationTarget(leaf, snapshot, viewId)
                  return (
                    <Box
                      key={`${leaf.connector.id}-${idx}`}
                      px={3}
                      py={2.5}
                      rounded="lg"
                      bg="whiteAlpha.50"
                      border="1px solid"
                      borderColor="whiteAlpha.100"
                      _hover={{ bg: 'whiteAlpha.100', borderColor: 'whiteAlpha.200' }}
                      transition="all 0.15s"
                    >
                      <VStack align="stretch" spacing={2}>
                        {/* Connector identity row */}
                        <HStack justify="space-between" align="center" spacing={1}>
                          {/* Source → Target */}
                          <HStack spacing={1} flex={1} minWidth={0} overflow="hidden">
                            <Text
                              color="white"
                              fontSize="xs"
                              fontWeight="semibold"
                              isTruncated
                              minWidth={0}
                              flex={1}
                              textAlign="right"
                            >
                              {truncate(leaf.source.actualElementName)}
                            </Text>
                            <Flex align="center" justify="center" flexShrink={0} px={1}>
                              <ConnectorArrowIcon direction={leaf.connector.direction} />
                            </Flex>
                            <Text
                              color="white"
                              fontSize="xs"
                              fontWeight="semibold"
                              isTruncated
                              minWidth={0}
                              flex={1}
                            >
                              {truncate(leaf.target.actualElementName)}
                            </Text>
                          </HStack>

                          {/* Edit / Delete actions */}
                          {canEdit && (
                            <HStack spacing={0.5} flexShrink={0}>
                              <IconButton
                                aria-label="Edit connector"
                                icon={<DrawIcon size={13} />}
                                size="xs"
                                variant="ghost"
                                color="blue.300"
                                _hover={{ bg: 'blue.900', color: 'blue.100' }}
                                onClick={(e) => {
                                  e.stopPropagation()
                                  onEdit?.(leaf.connector)
                                }}
                              />
                              <IconButton
                                aria-label="Delete connector"
                                icon={<TrashIcon size={13} />}
                                size="xs"
                                variant="ghost"
                                color="red.400"
                                _hover={{ bg: 'red.900', color: 'red.100' }}
                                onClick={(e) => {
                                  e.stopPropagation()
                                  onDelete?.(leaf.connector.id, leaf.ownerViewId)
                                }}
                              />
                            </HStack>
                          )}
                        </HStack>

                        {/* Label / relationship */}
                        {(leaf.connector.label || leaf.connector.relationship) && (
                          <Text
                            color="gray.400"
                            fontSize="xs"
                            fontStyle={!leaf.connector.label ? 'italic' : 'normal'}
                            noOfLines={1}
                          >
                            {leaf.connector.label || leaf.connector.relationship}
                          </Text>
                        )}

                        {/* Description */}
                        {leaf.connector.description && (
                          <Text
                            color="gray.500"
                            fontSize="xs"
                            lineHeight="1.5"
                            noOfLines={3}
                          >
                            {leaf.connector.description}
                          </Text>
                        )}

                        {/* Navigation button */}
                        {navigationTarget && (
                          <Button
                            size="xs"
                            variant="clay"
                            colorScheme="blue"
                            color="blue.100"
                            leftIcon={<NavigationIcon size={11} />}
                            onClick={(e) => {
                              e.preventDefault()
                              e.stopPropagation()
                              navigate(`/views/${navigationTarget.viewId}`)
                              onClose()
                            }}
                            w="full"
                            justifyContent="flex-start"
                            h="26px"
                            fontSize="11px"
                            mt={0.5}
                          >
                            Open {navigationTarget.viewName}
                          </Button>
                        )}
                      </VStack>
                    </Box>
                  )
                })}
              </VStack>
            </VStack>
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
