import { useEffect, useState } from 'react'
import {
  Box,
  Button,
  HStack,
  Popover,
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
  useDisclosure,
  VStack,
} from '@chakra-ui/react'
import { FocusIcon, ChevronDownIcon } from './Icons'
import {
  CROSS_BRANCH_CONNECTOR_BUDGET_MAX,
  CROSS_BRANCH_CONNECTOR_BUDGET_MIN,
} from '../crossBranch/types'
import type { CrossBranchConnectorPriority, CrossBranchContextSettings } from '../crossBranch/types'

const DENSITY_STOPS = [
  { value: -2, label: 'Quiet', budget: CROSS_BRANCH_CONNECTOR_BUDGET_MIN },
  { value: -1, label: 'Lean', budget: 25 },
  { value: 0, label: 'Normal', budget: 50 },
  { value: 1, label: 'Rich', budget: 100 },
  { value: 2, label: 'Full', budget: CROSS_BRANCH_CONNECTOR_BUDGET_MAX },
] as const

function densityFromBudget(budget: number) {
  return DENSITY_STOPS.reduce((closest, stop) => (
    Math.abs(stop.budget - budget) < Math.abs(closest.budget - budget) ? stop : closest
  ), DENSITY_STOPS[0]).value
}

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
  const { isOpen, onClose, onToggle } = useDisclosure()
  const [draftDensityLevel, setDraftDensityLevel] = useState<number>(() => densityFromBudget(connectorBudget))
  const activeDensity = DENSITY_STOPS.find((stop) => stop.value === draftDensityLevel) ?? DENSITY_STOPS[2]

  useEffect(() => {
    setDraftDensityLevel(densityFromBudget(connectorBudget))
  }, [connectorBudget])

  return (
    <Popover isOpen={isOpen} onClose={onClose} placement="top-start" isLazy closeOnBlur>
      <PopoverTrigger>
        <Button
          variant="ghost"
          h="28px"
          px={2.5}
          color={isOpen || settings.enabled ? 'var(--accent)' : 'gray.300'}
          bg={settings.enabled ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
          _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
          onClick={onToggle}
          aria-label={`Open ${label} filters`}
        >
          <HStack spacing={1.5}>
            <FocusIcon />
            <Text fontSize="11px" fontWeight={settings.enabled ? 'semibold' : 'normal'}>{label}</Text>
            <Text fontSize="10px" color={settings.enabled ? 'var(--accent)' : 'gray.400'}>
              {settings.enabled ? activeDensity.label : 'Off'}
            </Text>
            <ChevronDownIcon size={10} strokeWidth={3.5} />
          </HStack>
        </Button>
      </PopoverTrigger>
      <Portal>
        <PopoverContent
          bg="linear-gradient(180deg, rgba(var(--bg-main-rgb), 0.98) 0%, rgba(var(--bg-main-rgb), 0.92) 100%)"
          backdropFilter="blur(22px)"
          borderColor="whiteAlpha.100"
          boxShadow="0 18px 48px rgba(0,0,0,0.46), inset 0 1px 0 rgba(255,255,255,0.04)"
          borderRadius="lg"
          width="292px"
          _focus={{ boxShadow: 'none' }}
        >
          <PopoverBody p={3}>
            <VStack align="stretch" spacing={3}>
              <HStack
                justify="space-between"
                spacing={3}
                px={2.5}
                py={2}
                rounded="md"
                bg={settings.enabled ? 'rgba(var(--accent-rgb), 0.10)' : 'whiteAlpha.50'}
                border="1px solid"
                borderColor={settings.enabled ? 'rgba(var(--accent-rgb), 0.22)' : 'whiteAlpha.100'}
              >
                <HStack spacing={2.5} minW={0}>
                  <Box color={settings.enabled ? 'var(--accent)' : 'gray.400'} flexShrink={0}>
                    <FocusIcon />
                  </Box>
                  <Box minW={0}>
                    <Text fontSize="xs" fontWeight="semibold" color="whiteAlpha.900">Cross-branch context</Text>
                    <Text fontSize="10px" color="whiteAlpha.600" noOfLines={1}>
                      {settings.enabled ? 'Show relationships across branches' : 'Branch context is hidden'}
                    </Text>
                  </Box>
                </HStack>
                <Switch
                  size="sm"
                  isChecked={settings.enabled}
                  onChange={(event) => onEnabledChange(event.target.checked)}
                  colorScheme="teal"
                  flexShrink={0}
                  aria-label="Toggle cross-branch context"
                />
              </HStack>

              <Box
                opacity={settings.enabled ? 1 : 0.45}
                px={2.5}
                py={2.5}
                rounded="md"
                bg="whiteAlpha.50"
                border="1px solid"
                borderColor="whiteAlpha.100"
              >
                <HStack justify="space-between" mb={2}>
                  <Box>
                    <Text fontSize="xs" fontWeight="semibold" color="whiteAlpha.900">Priority</Text>
                    <Text fontSize="10px" color="whiteAlpha.600">Choose connector type to prioritize</Text>
                  </Box>
                </HStack>
                <HStack spacing={1} bg="rgba(255,255,255,0.06)" borderRadius="md" p={1}>
                  {(['external', 'internal'] as const).map((priority) => (
                    <Button
                      key={priority}
                      size="xs"
                      h="24px"
                      flex={1}
                      isDisabled={!settings.enabled}
                      variant="ghost"
                      bg={settings.connectorPriority === priority ? 'rgba(var(--accent-rgb), 0.18)' : 'transparent'}
                      color={settings.connectorPriority === priority ? 'var(--accent)' : 'gray.300'}
                      _hover={{ bg: settings.connectorPriority === priority ? 'rgba(var(--accent-rgb), 0.22)' : 'whiteAlpha.100' }}
                      onClick={() => onPriorityChange(priority)}
                    >
                      {priority === 'external' ? 'External' : 'Internal'}
                    </Button>
                  ))}
                </HStack>
              </Box>

              <Box
                opacity={settings.enabled ? 1 : 0.45}
                px={2.5}
                py={2.5}
                rounded="md"
                bg="whiteAlpha.50"
                border="1px solid"
                borderColor="whiteAlpha.100"
              >
                <HStack justify="space-between" mb={2.5}>
                  <Box>
                    <Text fontSize="xs" fontWeight="semibold" color="whiteAlpha.900">Density</Text>
                  </Box>
                  <Text
                    fontSize="10px"
                    fontWeight="bold"
                    color="var(--accent)"
                    bg="rgba(var(--accent-rgb), 0.10)"
                    border="1px solid"
                    borderColor="rgba(var(--accent-rgb), 0.18)"
                    rounded="full"
                    px={2}
                    py={0.5}
                  >
                    {activeDensity.label}
                  </Text>
                </HStack>
                <Box px={1} pt={1} pb={0.5}>
                  <Slider
                    aria-label="Branch density"
                    isDisabled={!settings.enabled}
                    min={-2}
                    max={2}
                    step={1}
                    value={draftDensityLevel}
                    onChange={setDraftDensityLevel}
                    onChangeEnd={(value) => {
                      setDraftDensityLevel(value)
                      const next = DENSITY_STOPS.find((stop) => stop.value === value) ?? DENSITY_STOPS[2]
                      onBudgetChange(next.budget)
                    }}
                    focusThumbOnChange={false}
                  >
                    <SliderTrack h="4px" bg="whiteAlpha.200">
                      <SliderFilledTrack bg="var(--accent)" />
                    </SliderTrack>
                    {DENSITY_STOPS.map((stop) => (
                      <Box
                        key={stop.value}
                        position="absolute"
                        left={`${((stop.value + 2) / 4) * 100}%`}
                        top="50%"
                        transform="translate(-50%, -50%)"
                        w={stop.value === draftDensityLevel ? '6px' : '2px'}
                        h={stop.value === draftDensityLevel ? '6px' : '10px'}
                        rounded="full"
                        bg={draftDensityLevel >= stop.value ? 'var(--accent)' : 'whiteAlpha.500'}
                        pointerEvents="none"
                      />
                    ))}
                    <SliderThumb boxSize="14px" bg="white" border="2px solid" borderColor="var(--accent)" />
                  </Slider>
                  <HStack justify="space-between" mt={2} px={0.5}>
                    {DENSITY_STOPS.map((stop) => (
                      <Text
                        key={stop.value}
                        fontSize="9px"
                        fontWeight={stop.value === draftDensityLevel ? 'bold' : 'medium'}
                        color={stop.value === draftDensityLevel ? 'whiteAlpha.900' : 'whiteAlpha.500'}
                      >
                        {stop.label}
                      </Text>
                    ))}
                  </HStack>
                  <Text fontSize="10px" color="whiteAlpha.500" mt={2}>
                    Connector budget {connectorBudget}
                  </Text>
                </Box>
              </Box>
            </VStack>
          </PopoverBody>
        </PopoverContent>
      </Portal>
    </Popover>
  )
}
