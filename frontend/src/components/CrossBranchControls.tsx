import {
  Box,
  Button,
  HStack,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverContent,
  PopoverTrigger,
  Portal,
  Slider,
  SliderFilledTrack,
  SliderThumb,
  SliderTrack,
  Switch,
  Text,
  VStack,
} from '@chakra-ui/react'
import { CROSS_BRANCH_DEPTH_ALL, CROSS_BRANCH_DEPTH_MAX, CROSS_BRANCH_DEPTH_MIN } from '../crossBranch/types'
import type { CrossBranchContextSettings } from '../crossBranch/types'

interface Props {
  settings: CrossBranchContextSettings
  onEnabledChange: (enabled: boolean) => void
  onDepthChange: (depth: number) => void
  label?: string
}

function depthLabel(depth: number) {
  return depth >= CROSS_BRANCH_DEPTH_ALL ? 'All' : String(depth)
}

export default function CrossBranchControls({
  settings,
  onEnabledChange,
  onDepthChange,
  label = 'Cross-Branch',
}: Props) {
  return (
    <Popover placement="top-start" isLazy>
      <PopoverTrigger>
        <Button
          variant="ghost"
          h="28px"
          px={2.5}
          color={settings.enabled ? 'var(--accent)' : 'gray.300'}
          bg={settings.enabled ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
          _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
        >
          <HStack spacing={1.5}>
            <Box w="7px" h="7px" rounded="full" bg={settings.enabled ? 'var(--accent)' : 'gray.500'} />
            <Text fontSize="11px" fontWeight="normal">{label}</Text>
            <Text fontSize="10px" color="gray.400">{settings.enabled ? depthLabel(settings.depth) : 'Off'}</Text>
          </HStack>
        </Button>
      </PopoverTrigger>
      <Portal>
        <PopoverContent
          bg="glass.bg"
          backdropFilter="blur(16px)"
          borderColor="glass.border"
          boxShadow="panel"
          borderRadius="lg"
          width="240px"
          _focus={{ boxShadow: 'none' }}
        >
          <PopoverArrow bg="glass.bg" />
          <PopoverBody p={3}>
            <VStack align="stretch" spacing={3}>
              <HStack justify="space-between">
                <Text fontSize="xs" fontWeight="600" color="white">Show cross-branch context</Text>
                <Switch isChecked={settings.enabled} onChange={(event) => onEnabledChange(event.target.checked)} colorScheme="blue" />
              </HStack>
              <Box opacity={settings.enabled ? 1 : 0.4}>
                <HStack justify="space-between" mb={2}>
                  <Text fontSize="10px" fontWeight="700" color="gray.400" letterSpacing="0.08em" textTransform="uppercase">
                    Descendant Depth
                  </Text>
                  <Text fontSize="xs" color="gray.300">{depthLabel(settings.depth)}</Text>
                </HStack>
                <Slider
                  isDisabled={!settings.enabled}
                  min={CROSS_BRANCH_DEPTH_MIN}
                  max={CROSS_BRANCH_DEPTH_MAX}
                  step={1}
                  value={settings.depth}
                  onChange={onDepthChange}
                >
                  <SliderTrack bg="whiteAlpha.200">
                    <SliderFilledTrack bg="var(--accent)" />
                  </SliderTrack>
                  <SliderThumb boxSize={4} />
                </Slider>
                <HStack justify="space-between" mt={1}>
                  <Text fontSize="10px" color="gray.500">Near</Text>
                  <Text fontSize="10px" color="gray.500">All</Text>
                </HStack>
              </Box>
            </VStack>
          </PopoverBody>
        </PopoverContent>
      </Portal>
    </Popover>
  )
}
