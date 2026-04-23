import { Box, Button, HStack, Icon, Text, VStack, Divider, Flex } from '@chakra-ui/react'
import { useNavigate } from 'react-router-dom'
import type { ProxyConnectorDetails } from '../crossBranch/types'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import { ChevronRightIcon, NavigationIcon } from './Icons'

interface Props {
  isOpen: boolean
  onClose: () => void
  details: ProxyConnectorDetails | null
  hasBackdrop?: boolean
}

export default function ProxyConnectorPanel({
  isOpen,
  onClose,
  details,
  hasBackdrop = true,
}: Props) {
  const navigate = useNavigate()

  return (
    <SlidingPanel
      isOpen={isOpen}
      onClose={onClose}
      panelKey="proxy-connector-panel"
      width={{ base: 'calc(100vw - 24px)', md: '300px' }}
      hasBackdrop={hasBackdrop}
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
                        <HStack spacing={2} mb={1}>
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
