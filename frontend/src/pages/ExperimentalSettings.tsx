import {
  Box,
  FormLabel,
  VStack,
  Checkbox,
  Text,
  HStack,
} from '@chakra-ui/react'
import { useExperimental } from '../context/ExperimentalContext'

export default function ExperimentalSettings({ compact = false }: { compact?: boolean }) {
  const { experimental, toggleExperimental } = useExperimental()
  const sectionGap = compact ? 4 : 6

  return (
    <VStack align="start" spacing={sectionGap} maxW={compact ? '320px' : '480px'} w="full">
      <Box w="full">
        <FormLabel mb={3} fontSize={compact ? 'xs' : 'sm'} textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
          Experimental
        </FormLabel>

        <HStack spacing={2} align="center">
          <Checkbox
            size="sm"
            colorScheme="blue"
            isChecked={experimental.populateEnabled}
            onChange={() => toggleExperimental('populateEnabled')}
          >
            <Text fontSize="sm" color="gray.200" userSelect="none">
              Populate elements
            </Text>
          </Checkbox>
        </HStack>
      </Box>
    </VStack>
  )
}
