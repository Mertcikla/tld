import { Box, Button, Grid, HStack, Text, VStack, Divider, Flex, IconButton, Tooltip } from '@chakra-ui/react'
import type { ReactNode, SVGProps } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ProxyConnectorDetails, ProxyConnectorLeaf, ProxyEndpoint, WorkspaceGraphSnapshot } from '../crossBranch/types'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import { NavigationIcon, TrashIcon, DrawIcon } from './Icons'
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

function ConnectorArrowIcon({ direction }: { direction: string }) {
  const strokeColor = 'var(--accent)'
  const commonProps: SVGProps<SVGSVGElement> = {
    width: '42',
    height: '14',
    viewBox: '0 0 42 14',
    fill: 'none',
    style: { display: 'block' },
  }

  switch (direction) {
    case 'backward':
      return (
        <svg {...commonProps}>
          <line x1="10" y1="7" x2="35" y2="7" stroke={strokeColor} strokeWidth="2" />
          <polygon points="10,3 3,7 10,11" fill={strokeColor} />
          <circle cx="35" cy="7" r="2.75" fill={strokeColor} />
        </svg>
      )
    case 'both':
      return (
        <svg {...commonProps}>
          <line x1="10" y1="7" x2="32" y2="7" stroke={strokeColor} strokeWidth="2" />
          <polygon points="10,3 3,7 10,11" fill={strokeColor} />
          <polygon points="32,3 39,7 32,11" fill={strokeColor} />
        </svg>
      )
    case 'none':
      return (
        <svg {...commonProps}>
          <line x1="7" y1="7" x2="35" y2="7" stroke={strokeColor} strokeWidth="2" />
          <circle cx="7" cy="7" r="2.75" fill={strokeColor} />
          <circle cx="35" cy="7" r="2.75" fill={strokeColor} />
        </svg>
      )
    case 'forward':
    default:
      return (
        <svg {...commonProps}>
          <line x1="7" y1="7" x2="32" y2="7" stroke={strokeColor} strokeWidth="2" />
          <circle cx="7" cy="7" r="2.75" fill={strokeColor} />
          <polygon points="32,3 39,7 32,11" fill={strokeColor} />
        </svg>
      )
  }
}

function ConnectorEndpointText({
  children,
  align = 'left',
  size = 'xs',
}: {
  children: string
  align?: 'left' | 'right'
  size?: 'xs' | 'sm'
}) {
  return (
    <Tooltip label={children} placement="top" openDelay={300} hasArrow>
      <Text
        color="white"
        fontSize={size}
        fontWeight="semibold"
        letterSpacing="0"
        lineHeight="1.2"
        isTruncated
        minWidth={0}
        textAlign={align}
      >
        {children}
      </Text>
    </Tooltip>
  )
}

function ConnectorIdentityGrid({
  source,
  target,
  direction,
  size = 'xs',
  actions,
}: {
  source: string
  target: string
  direction: string
  size?: 'xs' | 'sm'
  actions?: ReactNode
}) {
  return (
    <Grid
      templateColumns={actions ? 'minmax(0, 1fr) 42px minmax(0, 1fr) auto' : 'minmax(0, 1fr) 42px minmax(0, 1fr)'}
      alignItems="center"
      columnGap={3}
      w="full"
      minW={0}
    >
      <ConnectorEndpointText align="right" size={size}>{source}</ConnectorEndpointText>
      <Flex align="center" justify="center" w="42px" flexShrink={0}>
        <ConnectorArrowIcon direction={direction} />
      </Flex>
      <ConnectorEndpointText size={size}>{target}</ConnectorEndpointText>
      {actions && (
        <HStack spacing={0.5} flexShrink={0}>
          {actions}
        </HStack>
      )}
    </Grid>
  )
}

function SummaryConnectorLine() {
  return (
    <Box
      aria-hidden="true"
      w="56px"
      h="2px"
      rounded="full"
      bg="var(--accent)"
    />
  )
}

function SummaryConnectorIdentity({
  source,
  target,
}: {
  source: string
  target: string
}) {
  return (
    <Grid
      templateColumns="minmax(0, 1fr) 56px minmax(0, 1fr)"
      alignItems="center"
      columnGap={4}
      w="full"
      minW={0}
    >
      <Tooltip label={source} placement="top" openDelay={300} hasArrow>
        <Text
          color="white"
          fontSize="lg"
          fontWeight="700"
          lineHeight="1.15"
          isTruncated
          minW={0}
          textAlign="right"
        >
          {source}
        </Text>
      </Tooltip>
      <Flex align="center" justify="center" w="56px" flexShrink={0}>
        <SummaryConnectorLine />
      </Flex>
      <Tooltip label={target} placement="top" openDelay={300} hasArrow>
        <Text
          color="white"
          fontSize="lg"
          fontWeight="700"
          lineHeight="1.15"
          isTruncated
          minW={0}
        >
          {target}
        </Text>
      </Tooltip>
    </Grid>
  )
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

      <Box flex={1} overflowY="auto" py={0}>
        {details ? (
          <VStack align="stretch" spacing={0}>
            {/* Summary header */}
            <Box px={4} py={5}>
              <SummaryConnectorIdentity
                source={details.sourceAnchorName}
                target={details.targetAnchorName}
              />
            </Box>

            <Divider borderColor="whiteAlpha.100" />

            <VStack align="stretch" spacing={3} px={2} pt={3} pb={3}>
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
                      px={2}
                      py={2}
                      rounded="lg"
                      bg="whiteAlpha.50"
                      border="1px solid"
                      borderColor="whiteAlpha.100"
                      minH="54px"
                      display="flex"
                      alignItems="center"
                      _hover={{
                        bg: 'whiteAlpha.100',
                        borderColor: 'whiteAlpha.300',
                        transform: 'translateY(-1px)',
                        boxShadow: '0 4px 12px rgba(0,0,0,0.4)',
                      }}
                      transition="all 0.15s ease"
                    >
                      <VStack align="stretch" spacing={2} w="full" minW={0}>
                        {/* Connector identity row */}
                        <ConnectorIdentityGrid
                          source={leaf.source.actualElementName}
                          target={leaf.target.actualElementName}
                          direction={leaf.connector.direction}
                          actions={canEdit ? (
                            <>
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
                            </>
                          ) : undefined}
                        />

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
