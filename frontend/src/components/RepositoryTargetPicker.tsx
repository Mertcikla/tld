import { ChevronDownIcon } from '@chakra-ui/icons'
import type { ReactNode } from 'react'
import {
  Button,
  Menu,
  MenuButton,
  MenuItemOption,
  MenuList,
  MenuOptionGroup,
  Portal,
  Text,
} from '@chakra-ui/react'

type PickerOption = { value: string; label: string }
type PickerGroup = { label?: string; options: PickerOption[] }

export default function RepositoryTargetPicker({
  value,
  groups,
  onChange,
  isDisabled,
  'aria-label': ariaLabel,
  'data-testid': testId,
  triggerContent,
  compact = false,
  card = false,
}: {
  value: string
  groups: PickerGroup[]
  onChange: (value: string) => void
  isDisabled?: boolean
  'aria-label': string
  'data-testid'?: string
  triggerContent?: ReactNode
  compact?: boolean
  card?: boolean
}) {
  const label = groups.flatMap((group) => group.options).find((option) => option.value === value)?.label ?? value
  return (
    <Menu matchWidth placement="bottom-start" isLazy>
      <MenuButton
        as={Button}
        aria-label={ariaLabel}
        data-testid={testId}
        isDisabled={isDisabled}
        size={compact ? 'sm' : 'xs'}
        h={card ? '64px' : compact ? 'auto' : '28px'}
        minH={compact ? '24px' : '28px'}
        minW={compact ? '280px' : undefined}
        w={compact && !card ? 'auto' : 'full'}
        px={card ? 3 : compact ? 0 : 2}
        variant={compact && !card ? 'ghost' : 'outline'}
        bg={card ? 'whiteAlpha.50' : compact ? 'transparent' : 'var(--bg-panel)'}
        borderColor={compact && !card ? 'transparent' : 'whiteAlpha.200'}
        borderRadius={card ? 'lg' : undefined}
        fontWeight="normal"
        textAlign="left"
        rightIcon={compact && !card ? undefined : <ChevronDownIcon color="gray.400" />}
        _hover={{ bg: 'whiteAlpha.100', ...(card ? { borderColor: 'whiteAlpha.400' } : {}) }}
        _expanded={{ borderColor: 'var(--accent)', bg: 'whiteAlpha.100' }}
      >
        {triggerContent ?? <Text as="span" display="block" isTruncated title={label}>{label}</Text>}
      </MenuButton>
      <Portal>
        <MenuList
          minW={compact ? '280px' : 0}
          maxH="280px"
          overflowY="auto"
          overscrollBehavior="contain"
          bg="var(--bg-panel)"
          borderColor="whiteAlpha.200"
          borderRadius="md"
          py={1}
          boxShadow="lg"
        >
          <MenuOptionGroup type="radio" value={value} onChange={(next) => onChange(next as string)}>
            {groups.filter((group) => group.options.length).flatMap((group, index) => [
              ...(group.label ? [
                <Text key={`group-${index}`} px={3} pt={2} pb={1} fontSize="2xs" fontWeight="semibold" color="gray.500">
                  {group.label}
                </Text>,
              ] : []),
              ...group.options.map((option) => (
                <MenuItemOption
                  key={option.value}
                  value={option.value}
                  minH="28px"
                  py={1}
                  px={2}
                  fontSize="xs"
                  bg={option.value === value ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
                  _focus={{ bg: 'whiteAlpha.100' }}
                  _hover={{ bg: 'whiteAlpha.100' }}
                >
                  <Text isTruncated title={option.label}>{option.label}</Text>
                </MenuItemOption>
              )),
            ])}
          </MenuOptionGroup>
        </MenuList>
      </Portal>
    </Menu>
  )
}
