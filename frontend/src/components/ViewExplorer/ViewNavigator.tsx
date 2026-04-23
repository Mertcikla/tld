import React from 'react'
import { Box, Tooltip, VStack, Text, Divider } from '@chakra-ui/react'
import { ZoomInIcon, ZoomOutIcon, ChevronDownIcon } from '../Icons'
import { KbdHint } from '../PanelUI'
import { NavItem } from './types'
import { PARENT_VIEW_COLOR, CHILD_VIEW_COLOR } from '../../constants/colors'

interface Props {
  parents: NavItem[]
  children: NavItem[]
  activeFilter: 'out' | 'in' | null
  onFilterToggle: (type: 'out' | 'in', items: NavItem[]) => void
  onHoverZoom?: (elementId: number | null, type: 'in' | 'out' | null) => void
}

export const ViewNavigator: React.FC<Props> = ({
  parents,
  children,
  activeFilter,
  onFilterToggle,
  onHoverZoom,
}) => {
  const renderNavButton = (type: 'out' | 'in', items: NavItem[]) => {
    const isOut = type === 'out'
    const label = isOut ? 'Zoom Out' : 'Zoom In'
    const IconCmp = isOut ? ZoomOutIcon : ZoomInIcon
    const shortcut = isOut ? 'W' : 'S'
    const disabled = items.length === 0
    const isActive = activeFilter === type
    const accentColor = isOut ? PARENT_VIEW_COLOR : CHILD_VIEW_COLOR

    const subtitle = disabled
      ? isOut
        ? 'No parent views'
        : 'No child views'
      : items.length === 1
        ? items[0].name
        : `Select from ${items.length} options`

    return (
      <Tooltip
        label={
          disabled
            ? isOut
              ? 'No parent views'
              : 'No child views'
            : `Navigate to ${isOut ? 'Parent' : 'Child'} View [${shortcut}]`
        }
        placement="left"
        openDelay={400}
      >
        <Box
          as="button"
          role="group"
          className={`panel-action-button ${isActive ? 'is-active' : ''}`}
          disabled={disabled}
          onClick={() => onFilterToggle(type, items)}
          onMouseEnter={() => {
            if (disabled || items.length !== 1) return
            onHoverZoom?.(items[0].elementId ?? null, type)
          }}
          onMouseLeave={() => {
            if (disabled || items.length !== 1) return
            onHoverZoom?.(null, null)
          }}
          opacity={disabled ? 0.4 : 1}
        >
          <Box className="panel-action-icon-container" color={disabled ? 'gray.600' : accentColor}>
            <IconCmp />
          </Box>
          <VStack align="start" spacing={0} flex={1} minW={0}>
            <Text
              fontSize="sm"
              color={disabled ? 'gray.500' : 'white'}
              fontWeight="medium"
              isTruncated
              w="full"
              textAlign="left"
            >
              {label}
            </Text>
            <Text
              fontSize="xs"
              color={disabled ? 'gray.600' : isActive ? accentColor : 'gray.400'}
              isTruncated
              w="full"
              transition="color 0.15s"
              textAlign="left"
            >
              {subtitle}
            </Text>
          </VStack>
          {items.length > 1 && (
            <Box
              color="whiteAlpha.400"
              _groupHover={{ color: 'white' }}
              flexShrink={0}
              transform={isActive ? 'rotate(180deg)' : 'none'}
              transition="all 0.25s cubic-bezier(0.25, 1, 0.5, 1)"
              mx={1}
            >
              <ChevronDownIcon size={12} strokeWidth={3.5} />
            </Box>
          )}
          {isActive && items.length <= 1 && (
            <Box w="5px" h="5px" rounded="full" bg={accentColor} flexShrink={0} mx={1} />
          )}
          <KbdHint>{shortcut}</KbdHint>
        </Box>
      </Tooltip>
    )
  }

  return (
    <VStack spacing={0} align="stretch" flexShrink={0} py={1}>
      <Box>{renderNavButton('out', parents)}</Box>
      <Divider borderColor="whiteAlpha.100" />
      <Box>{renderNavButton('in', children)}</Box>
    </VStack>
  )
}
