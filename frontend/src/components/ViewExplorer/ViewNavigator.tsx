import React, { useState } from 'react'
import { Box, Tooltip, VStack, Text, HStack } from '@chakra-ui/react'
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
  const [hoveredType, setHoveredType] = useState<'out' | 'in' | null>(null)

  const renderNavButton = (type: 'out' | 'in', items: NavItem[]) => {
    const isOut = type === 'out'
    const label = isOut ? 'Zoom Out' : 'Zoom In'
    const IconCmp = isOut ? ZoomOutIcon : ZoomInIcon
    const shortcut = isOut ? 'W' : 'S'
    const disabled = items.length === 0
    const isActive = activeFilter === type
    const accentColor = isOut ? PARENT_VIEW_COLOR : CHILD_VIEW_COLOR
    const isHovered = hoveredType === type

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
        placement="top"
        openDelay={400}
      >
        <Box
          as="button"
          role="group"
          className={`panel-action-button ${isActive ? 'is-active' : ''}`}
          disabled={disabled}
          onClick={() => onFilterToggle(type, items)}
          onMouseEnter={() => {
            setHoveredType(type)
            if (disabled || items.length !== 1) return
            onHoverZoom?.(items[0].elementId ?? null, type)
          }}
          onMouseLeave={() => {
            setHoveredType(null)
            if (disabled || items.length !== 1) return
            onHoverZoom?.(null, null)
          }}
          opacity={disabled ? 0.4 : 1}
          flex={hoveredType === null ? 1 : (isHovered ? 4 : 1)}
          transition="all 0.4s cubic-bezier(0.4, 0, 0.2, 1)"
          minW={0}
          overflow="hidden"
          position="relative"
          px={isHovered || hoveredType === null ? 3 : 1}
          sx={{
            '.panel-action-icon-container': {
              transition: 'all 0.3s ease',
              width: '28px',
              height: '28px',
              marginRight: isOut ? '0.5rem' : 0,
              marginLeft: isOut ? 0 : '0.5rem',
              position: 'relative',
            }
          }}
        >
          {isOut ? (
            <>
              <Box className="panel-action-icon-container" color={disabled ? 'gray.600' : accentColor}>
                <IconCmp />
                {items.length > 1 && (
                  <Box
                    position="absolute"
                    bottom="-5px"
                    right="-5px"
                    color="white"
                    fontSize="12px"
                    fontWeight="bold"
                    minW="14px"
                    h="14px"
                    display="flex"
                    alignItems="center"
                    justifyContent="center"
                    zIndex={2}
                  >
                    {items.length}
                  </Box>
                )}
              </Box>
              <VStack
                align="start"
                spacing={0}
                flex={1}
                minW={0}
                opacity={hoveredType === 'in' ? 0 : 1}
                transition="opacity 0.2s, transform 0.3s"
                transform={hoveredType === 'in' ? 'translateX(-10px)' : 'none'}
              >
                <Text fontSize="xs" color={disabled ? 'gray.500' : 'white'} fontWeight="bold" isTruncated w="full" textAlign="left">
                  {label}
                </Text>
                {isHovered && (
                  <Text fontSize="10px" color={disabled ? 'gray.600' : isActive ? accentColor : 'gray.500'} isTruncated w="full" transition="color 0.15s" textAlign="left">
                    {subtitle}
                  </Text>
                )}
              </VStack>
              <Box opacity={hoveredType === 'in' ? 0 : 1} transition="opacity 0.2s">
                <KbdHint ml={1}>{shortcut}</KbdHint>
              </Box>
            </>
          ) : (
            <>
              <Box opacity={hoveredType === 'out' ? 0 : 1} transition="opacity 0.2s">
                <KbdHint ml={0} mr={1}>{shortcut}</KbdHint>
              </Box>
              <VStack
                align="end"
                spacing={0}
                flex={1}
                minW={0}
                opacity={hoveredType === 'out' ? 0 : 1}
                transition="opacity 0.2s, transform 0.3s"
                transform={hoveredType === 'out' ? 'translateX(10px)' : 'none'}
              >
                <Text fontSize="xs" color={disabled ? 'gray.500' : 'white'} fontWeight="bold" isTruncated w="full" textAlign="right">
                  {label}
                </Text>
                {isHovered && (
                  <Text fontSize="10px" color={disabled ? 'gray.600' : isActive ? accentColor : 'gray.500'} isTruncated w="full" transition="color 0.15s" textAlign="right">
                    {subtitle}
                  </Text>
                )}
              </VStack>
              <Box className="panel-action-icon-container" color={disabled ? 'gray.600' : accentColor}>
                <IconCmp />
                {items.length > 1 && (
                  <Box
                    position="absolute"
                    bottom="-5px"
                    right="-5px"
                    color="white"
                    fontSize="12px"
                    fontWeight="bold"
                    minW="14px"
                    h="14px"
                    display="flex"
                    alignItems="center"
                    justifyContent="center"
                    zIndex={2}
                  >
                    {items.length}
                  </Box>
                )}
              </Box>
            </>
          )}
          {items.length > 1 && isHovered && (
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
          {isActive && items.length <= 1 && isHovered && (
            <Box w="5px" h="5px" rounded="full" bg={accentColor} flexShrink={0} mx={1} />
          )}
        </Box>
      </Tooltip>
    )
  }

  return (
    <HStack spacing={0} align="stretch" flexShrink={0} height="60px" borderColor="whiteAlpha.100">
      {renderNavButton('out', parents)}
      <Box w="1px" bg="whiteAlpha.100" />
      {renderNavButton('in', children)}
    </HStack>
  )
}
