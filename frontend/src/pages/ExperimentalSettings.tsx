import {
  Box,
  FormLabel,
  VStack,
  Checkbox,
  Text,
  Link,
  HStack,
  Divider,
} from '@chakra-ui/react'
import { useExperimental } from '../context/ExperimentalContext'
import WatchControlPanel from '../components/watch/WatchControlPanel'

export interface ExperimentalSettingsProps {
  compact?: boolean
  // Which settings surface is rendering this section. Pane-only surfaces omit
  // the full watch configuration, which lives on the settings page.
  surface?: 'pane' | 'page'
}

export default function ExperimentalSettings({ compact = false, surface = 'page' }: ExperimentalSettingsProps) {
  const { experimental, toggleExperimental } = useExperimental()
  const sectionGap = compact ? 4 : 6
  const showWatchPanel = surface === 'page'

  return (
    <VStack align="start" spacing={sectionGap} maxW={compact ? '320px' : '720px'} w="full">
      <Box w="full">
        <FormLabel mb={3} fontSize={compact ? 'xs' : 'sm'} textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
          Experimental
        </FormLabel>

        <HStack spacing={2} align="center">
          <Checkbox
            size="sm"
            colorScheme="blue"
            isChecked={experimental.watchEnabled}
            onChange={() => toggleExperimental('watchEnabled')}
          >
            <Text fontSize="sm" color="gray.200" userSelect="none">
              Watch
            </Text>
          </Checkbox>
          <Link
            href="https://tldiagram.com/docs/tld/watch/"
            isExternal
            fontSize="xs"
            color="blue.300"
            _hover={{ color: 'blue.200', textDecoration: 'underline' }}
          >
            Docs
          </Link>
        </HStack>

        {!experimental.watchEnabled && (
          <Text mt={2} fontSize="xs" color="gray.500">
            Enable Watch to configure repositories, start scanning, and tune pipeline settings.
          </Text>
        )}
      </Box>

      {showWatchPanel && (
        <>
          <Divider borderColor="whiteAlpha.100" />

          <WatchControlPanel enabled={experimental.watchEnabled} compact={compact} />
        </>
      )}
    </VStack>
  )
}
