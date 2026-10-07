import {
  Badge,
  Box,
  Button,
  Flex,
  HStack,
  IconButton,
  Text,
  VStack,
} from '@chakra-ui/react'
import {
  faChevronDown,
  faChevronUp,
  faCircleCheck,
  faDownload,
  faTriangleExclamation,
  type IconDefinition,
} from '@fortawesome/free-solid-svg-icons'
import type { RepositoryIndexerCheck } from '../api/client'

function FaIcon({ icon, size = 13 }: { icon: IconDefinition; size?: number }) {
  const [width, height, , , pathData] = icon.icon
  const paths = Array.isArray(pathData) ? pathData : [pathData]
  return (
    <svg width={size} height={size} viewBox={`0 0 ${width} ${height}`} fill="currentColor" aria-hidden="true" style={{ flexShrink: 0 }}>
      {paths.map((path, index) => <path key={index} d={path} />)}
    </svg>
  )
}

export interface RepositoryIndexerChecklistProps {
  // check is null before the first scout of the selected repository.
  check: RepositoryIndexerCheck | null
  loading: boolean
  open: boolean
  onToggle: () => void
  onCheck: () => void
}

// RepositoryIndexerChecklist reports the external SCIP indexers a repository
// requires, their detected and minimum versions, and a download link for any
// that are missing or outdated. The section collapses once everything is green.
export default function RepositoryIndexerChecklist({ check, loading, open, onToggle, onCheck }: RepositoryIndexerChecklistProps) {
  if (!check) {
    return (
      <Flex align="center" gap={2} mb={3} data-testid="repositories-indexers">
        <Text fontSize="10px" fontWeight="bold" color="gray.500" textTransform="uppercase" letterSpacing="0.06em">
          Indexers
        </Text>
        <Box flex={1} />
        <Button
          size="xs"
          variant="outline"
          data-testid="repositories-indexers-check"
          isLoading={loading}
          onClick={onCheck}
        >
          Check indexers
        </Button>
      </Flex>
    )
  }
  if (check.indexers.length === 0) {
    return (
      <HStack spacing={1.5} color="green.300" mb={3} data-testid="repositories-indexers">
        <FaIcon icon={faCircleCheck} />
        <Text fontSize="xs">No external indexers required.</Text>
      </HStack>
    )
  }
  const missing = check.indexers.filter((indexer) => !indexer.installed)
  const outdated = check.indexers.filter((indexer) => indexer.installed && indexer.belowMinimum)
  return (
    <Box mb={3} border="1px solid" borderColor="whiteAlpha.100" borderRadius="md" data-testid="repositories-indexers">
      <Flex px={2} py={1.5} align="center" gap={2}>
        <IconButton
          size="xs"
          variant="ghost"
          aria-label={open ? 'Collapse indexer checks' : 'Expand indexer checks'}
          aria-expanded={open}
          data-testid="repositories-indexers-toggle"
          icon={<FaIcon icon={open ? faChevronUp : faChevronDown} size={11} />}
          onClick={onToggle}
        />
        <Text fontSize="xs" fontWeight="semibold">Indexers</Text>
        <Badge
          colorScheme={missing.length ? 'red' : outdated.length ? 'orange' : 'green'}
          fontSize="2xs"
          textTransform="none"
          data-testid="repositories-indexers-status"
        >
          {missing.length
            ? `${missing.length} missing`
            : outdated.length
              ? `${outdated.length} outdated`
              : 'Ready'}
        </Badge>
        <Box flex={1} />
        <Button
          size="xs"
          variant="ghost"
          color="gray.400"
          data-testid="repositories-indexers-recheck"
          isLoading={loading}
          onClick={onCheck}
        >
          Re-check
        </Button>
      </Flex>
      {open && (
        <VStack align="stretch" spacing={0} px={2} pb={2} maxH="220px" overflowY="auto">
          {check.indexers.map((indexer) => {
            const green = indexer.installed && !indexer.belowMinimum
            return (
              <HStack
                key={indexer.tool}
                align="flex-start"
                spacing={2}
                py={1.5}
                data-testid={`repositories-indexer-${indexer.tool}`}
              >
                <Box
                  flexShrink={0}
                  mt="2px"
                  color={green ? 'green.300' : indexer.installed ? 'orange.300' : 'red.300'}
                  aria-label={green ? `${indexer.tool} ready` : indexer.installed ? `${indexer.tool} outdated` : `${indexer.tool} missing`}
                >
                  <FaIcon icon={green ? faCircleCheck : faTriangleExclamation} />
                </Box>
                <Box minW={0} flex={1}>
                  <HStack spacing={2} wrap="wrap">
                    <Text fontSize="xs" fontWeight="semibold">{indexer.tool}</Text>
                    {indexer.version && (
                      <Text fontSize="2xs" color="gray.500" fontFamily="mono">{indexer.version}</Text>
                    )}
                    {indexer.minVersion && (
                      <Text fontSize="2xs" color={green ? 'gray.500' : 'orange.300'}>
                        {`min ${indexer.minVersion}`}
                      </Text>
                    )}
                  </HStack>
                  <Text fontSize="2xs" color="gray.500">
                    {indexer.languages.join(', ')}
                    {indexer.installed ? '' : ' · not installed'}
                    {indexer.belowMinimum ? ' · update recommended' : ''}
                  </Text>
                  {!green && indexer.installHint && (
                    <Text
                      as="code"
                      display="block"
                      mt={1}
                      px={1.5}
                      py={1}
                      borderRadius="sm"
                      bg="blackAlpha.100"
                      fontSize="2xs"
                      wordBreak="break-all"
                    >
                      {indexer.installHint}
                    </Text>
                  )}
                  {!green && indexer.downloadUrl && (
                    <Button
                      as="a"
                      href={indexer.downloadUrl}
                      target="_blank"
                      rel="noreferrer"
                      mt={1}
                      size="xs"
                      variant="outline"
                      leftIcon={<FaIcon icon={faDownload} size={10} />}
                      data-testid={`repositories-indexer-download-${indexer.tool}`}
                    >
                      Download
                    </Button>
                  )}
                </Box>
              </HStack>
            )
          })}
        </VStack>
      )}
    </Box>
  )
}
