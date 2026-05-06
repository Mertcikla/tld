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
import {
  CROSS_BRANCH_CONNECTOR_BUDGET_MAX,
  CROSS_BRANCH_CONNECTOR_BUDGET_MIN,
} from '../crossBranch/types'
import type { CrossBranchConnectorPriority, CrossBranchContextSettings } from '../crossBranch/types'

interface Props {
  settings: CrossBranchContextSettings
  onEnabledChange: (enabled: boolean) => void
  onBudgetChange: (budget: number) => void
  onPriorityChange: (priority: CrossBranchConnectorPriority) => void
  label?: string
}

export default function CrossBranchControls({
  settings,
  onEnabledChange,
  onBudgetChange,
  onPriorityChange,
  label = 'Cross-Branch',
}: Props) {
  const connectorBudget = settings.connectorBudget

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
            <Text fontSize="10px" color="gray.400">{settings.enabled ? connectorBudget : 'Off'}</Text>
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
                <Text fontSize="10px" fontWeight="700" color="gray.400" letterSpacing="0.08em" textTransform="uppercase" mb={2}>
                  Priority
                </Text>
                <HStack spacing={1} bg="whiteAlpha.100" borderRadius="md" p={1}>
                  {(['external', 'internal'] as const).map((priority) => {
                    const active = settings.connectorPriority === priority
                    return (
                      <Button
                        key={priority}
                        size="xs"
                        h="24px"
                        flex={1}
                        isDisabled={!settings.enabled}
                        variant="ghost"
                        bg={active ? 'rgba(var(--accent-rgb), 0.18)' : 'transparent'}
                        color={active ? 'var(--accent)' : 'gray.300'}
                        _hover={{ bg: active ? 'rgba(var(--accent-rgb), 0.22)' : 'whiteAlpha.100' }}
                        onClick={() => onPriorityChange(priority)}
                      >
                        {priority === 'external' ? 'External' : 'Internal'}
                      </Button>
                    )
                  })}
                </HStack>
              </Box>
              <Box opacity={settings.enabled ? 1 : 0.4}>
                <HStack justify="space-between" mb={2}>
                  <Text fontSize="10px" fontWeight="700" color="gray.400" letterSpacing="0.08em" textTransform="uppercase">
                    Connector Budget
                  </Text>
                  <Text fontSize="xs" color="gray.300">{connectorBudget}</Text>
                </HStack>
                <Slider
                  isDisabled={!settings.enabled}
                  min={CROSS_BRANCH_CONNECTOR_BUDGET_MIN}
                  max={CROSS_BRANCH_CONNECTOR_BUDGET_MAX}
                  step={1}
                  value={connectorBudget}
                  onChange={onBudgetChange}
                >
                  <SliderTrack bg="whiteAlpha.200">
                    <SliderFilledTrack bg="var(--accent)" />
                  </SliderTrack>
                  <SliderThumb boxSize={4} />
                </Slider>
                <HStack justify="space-between" mt={1}>
                  <Text fontSize="10px" color="gray.500">{CROSS_BRANCH_CONNECTOR_BUDGET_MIN}</Text>
                  <Text fontSize="10px" color="gray.500">{CROSS_BRANCH_CONNECTOR_BUDGET_MAX}</Text>
                </HStack>
              </Box>
            </VStack>
          </PopoverBody>
        </PopoverContent>
      </Portal>
    </Popover>
  )
}
