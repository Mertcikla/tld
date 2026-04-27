import { Box, Button, HStack, Icon, Text, VStack, Divider, Flex, IconButton } from '@chakra-ui/react'
import { useNavigate } from 'react-router-dom'
import type { ProxyConnectorDetails } from '../crossBranch/types'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import { ChevronRightIcon, NavigationIcon, TrashIcon, EditIcon } from './Icons'
import { useViewEditorContext } from '../pages/ViewEditor/context'
import type { Connector } from '../types'

interface Props {
  isOpen: boolean
  onClose: () => void
  details: ProxyConnectorDetails | null
  hasBackdrop?: boolean
  onEdit?: (connector: Connector) => void
  onDelete?: (connectorId: number, ownerViewId: number) => void
}

export default function ProxyConnectorPanel({
  isOpen,
  onClose,
  details,
  hasBackdrop = true,
  onEdit,
  onDelete,
}: Props) {
  const navigate = useNavigate()
  const { canEdit } = useViewEditorContext()

  return (
    <SlidingPanel
      isOpen={isOpen}
      onClose={onClose}
      panelKey="proxy-connector-panel"
      width={{ base: 'calc(100vw - 24px)', md: '300px' }}
      hasBackdrop={hasBackdrop}
      zIndex={950}
    >
      <PanelHeader title="Relationships" onClose={onClose} />

      <Box flex={1} overflowY="auto" px={4} py={4}>
        {details ? (
          <VStack align="stretch" spacing={6}>
            {/* Header info */}
            <Box>
              <HStack spacing={2} mb={1}>
                <Text color="white" fontSize="s" letterSpacing="-0.01em">
                  {details.sourceAnchorName}
                </Text>
                <Icon as={ChevronRightIcon} color="whiteAlpha.400" />
                <Text color="white" fontSize="s" letterSpacing="-0.01em">
                  {details.targetAnchorName}
                </Text>
              </HStack>
              <Text color="blue.300" fontSize="xs" fontWeight="600" textTransform="uppercase" letterSpacing="0.05em">
                {details.label}
              </Text>
            </Box>

            <Divider borderColor="whiteAlpha.100" />

            <VStack align="stretch" spacing={4}>
              <Text color="gray.500" fontSize="10px" fontWeight="800" letterSpacing="0.1em" textTransform="uppercase">
                Underlying Connectors
              </Text>

              <VStack align="stretch" spacing={3}>
                {details.connectors.map((leaf, idx) => (
                  <Box
                    key={`${leaf.connector.id}-${idx}`}
                    px={3}
                    py={3}
                    rounded="xl"
                    bg="whiteAlpha.50"
                    border="1px solid"
                    borderColor="whiteAlpha.100"
                    _hover={{ bg: 'whiteAlpha.100', borderColor: 'whiteAlpha.200' }}
                    transition="all 0.2s"
                  >
                    <VStack align="stretch" spacing={3}>
                      <Box>
                        <HStack justify="space-between" align="start">
                          <VStack align="start" spacing={1} flex={1}>
                            <HStack spacing={2}>
                              <Text color="white" fontSize="sm" fontWeight="semibold" isTruncated>
                                {leaf.source.actualElementName}
                              </Text>
                              <Icon as={ChevronRightIcon} color="whiteAlpha.400" />
                              <Text color="white" fontSize="sm" fontWeight="semibold" isTruncated>
                                {leaf.target.actualElementName}
                              </Text>
                            </HStack>

                            {(leaf.connector.label || leaf.connector.relationship) && (
                              <Text color="gray.400" fontSize="xs" fontStyle={!leaf.connector.label ? 'italic' : 'normal'}>
                                {leaf.connector.label || leaf.connector.relationship}
                              </Text>
                            )}
                          </VStack>

                          {canEdit && (
                            <HStack spacing={1} mt={-1}>
                              <IconButton
                                aria-label="Edit connector"
                                icon={<EditIcon size={14} />}
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
                                icon={<TrashIcon size={14} />}
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
                      </Box>

                      {leaf.connector.description && (
                        <Text color="gray.500" fontSize="xs" lineHeight="tall" pb={1}>
                          {leaf.connector.description}
                        </Text>
                      )}

                      <Button
                        size="xs"
                        variant="clay"
                        colorScheme="blue"
                        color="blue.100"
                        leftIcon={<NavigationIcon size={12} />}
                        onClick={(e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          navigate(`/views/${leaf.ownerViewId}`);
                          onClose();
                        }}
                        w="full"
                        justifyContent="flex-start"
                        h="28px"
                        fontSize="11px"
                      >
                        Open {leaf.ownerViewName}
                      </Button>
                    </VStack>
                  </Box>
                ))}
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

