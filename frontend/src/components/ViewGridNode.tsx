import { useEffect, useRef, useState } from 'react'
import { useAccentColor } from '../context/ThemeContext'
import { hexToRgba } from '../constants/colors'
import { Handle, Position } from 'reactflow'
import { keyframes } from '@emotion/react'
import {
  Box,
  Flex,
  IconButton,
  Input,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  Portal,
  Spinner,
  Text,
  Tooltip,
  useDisclosure,
} from '@chakra-ui/react'
import { api } from '../api/client'

const shimmer = keyframes`
  0%   { background-position: -260px 0 }
  100% { background-position: 260px 0 }
`

const badgePop = keyframes`
  from { opacity: 0; transform: scale(0.7); }
  to   { opacity: 1; transform: scale(1); }
`

// Badges float OUTSIDE the card in the gap between nodes - avoids competing with card content
const WASD_BADGE_POS = {
  w: { bottom: '-18px', left: 'calc(50% - 15px)' },
  s: { top: '-18px', left: 'calc(50% - 15px)' },
  a: { right: '-18px', top: 'calc(50% - 12px)' },
  d: { left: '-18px', top: 'calc(50% - 12px)' },
} as const

export interface ViewGridNodeData {
  id: number
  name: string
  level_label: string | null
  counts?: { nodes: number; edges: number }
  focused: boolean
  canEdit: boolean
  isEditing: boolean
  editName: string
  onFocus: () => void
  onOpen: () => void
  onStartRename: () => void
  onDetails: () => void
  onDelete: () => void
  onShare: () => void
  onEditNameChange: (v: string) => void
  onEditCommit: () => void
  onEditCancel: () => void
  /** On touch/mobile: tap navigates directly instead of just focusing */
  isMobile?: boolean
  /** Key that navigates to this card from the currently selected card */
  wasdKey?: 'w' | 'a' | 's' | 'd'
}

export default function ViewGridNode({ data }: { data: ViewGridNodeData }) {
  const inputRef = useRef<HTMLInputElement>(null)
  const boxRef = useRef<HTMLDivElement>(null)
  const { accent } = useAccentColor()
  const [thumbnailUrl, setThumbnailUrl] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [hasRequested, setHasRequested] = useState(false)

  const { isOpen: isMenuOpen, onOpen: onMenuOpen, onClose: onMenuClose } = useDisclosure()
  const [isTooltipOpen, setIsTooltipOpen] = useState(false)

  useEffect(() => {
    if (!isMenuOpen && !isTooltipOpen) return

    const closeFloatingUi = () => {
      onMenuClose()
      setIsTooltipOpen(false)
    }

    window.addEventListener('wheel', closeFloatingUi, { passive: true, capture: true })
    window.addEventListener('touchmove', closeFloatingUi, { passive: true, capture: true })

    return () => {
      window.removeEventListener('wheel', closeFloatingUi, { capture: true })
      window.removeEventListener('touchmove', closeFloatingUi, { capture: true })
    }
  }, [isMenuOpen, isTooltipOpen, onMenuClose])

  useEffect(() => {
    if (data.isEditing && inputRef.current) {
      inputRef.current.focus()
      inputRef.current.select()
    }
  }, [data.isEditing])

  // Trigger thumbnail request when card becomes visible
  useEffect(() => {
    if (!boxRef.current) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          setHasRequested(true)
          observer.disconnect()
        }
      },
      { threshold: 0.1 }
    )
    observer.observe(boxRef.current)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (!hasRequested) return

    let active = true
    let url: string | null = null

    const loadThumbnail = async () => {
      setIsLoading(true)
      try {
        url = await api.workspace.views.thumbnail(data.id)
        if (active && url) {
          setThumbnailUrl(url)
        }
      } catch (err) {
        console.error('Failed to load thumbnail:', err)
      } finally {
        if (active) setIsLoading(false)
      }
    }

    loadThumbnail()

    return () => {
      active = false
      if (url?.startsWith('blob:')) {
        URL.revokeObjectURL(url)
      }
    }
  }, [hasRequested, data.id])

  const borderColor = data.focused ? accent : 'rgba(255,255,255,0.14)'

  const boxShadow = data.focused
    ? `0 0 24px ${hexToRgba(accent, 0.4)}`
    : '0 8px 24px rgba(0,0,0,0.4), inset 0 1px 0 rgba(255,255,255,0.05)'

  return (
    // Outer container: sizing + group context, overflow visible for the "New Child" hover button
    // `nopan` class tells ReactFlow's d3-zoom to not start panning on mousedown within this node
    <Box
      ref={boxRef}
      role="group"
      className="nopan"
      w="260px"
      h="150px"
      position="relative"
      userSelect="none"
      transition="opacity 0.3s cubic-bezier(0.16, 1, 0.3, 1)"
    >
      <Handle
        type="target"
        position={Position.Top}
        style={{ opacity: 0, pointerEvents: 'none' }}
        isConnectable={false}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ opacity: 0, pointerEvents: 'none' }}
        isConnectable={false}
      />

      {/* Card surface - clipped to border radius */}
      <Box
        position="absolute"
        inset={0}
        bg="var(--bg-card-solid)"
        borderRadius="12px"
        border="1.5px solid"
        borderColor={borderColor}
        boxShadow={boxShadow}
        transform={data.focused ? 'scale(1.025) translateY(-2px)' : 'scale(1)'}
        cursor={data.canEdit ? 'pointer' : 'pointer'}
        transition="all 0.2s cubic-bezier(0.16, 1, 0.3, 1)"
        overflow="hidden"
        onClick={data.isMobile ? data.onOpen : data.onFocus}
        onDoubleClick={data.onOpen}
        onMouseDown={(e) => e.stopPropagation()}
        _groupHover={{
          borderColor: data.focused
            ? borderColor
            : data.canEdit ? 'rgba(255,255,255,0.3)' : 'rgba(255,255,255,0.22)',
          boxShadow: data.focused
            ? boxShadow
            : data.canEdit
              ? '0 14px 40px rgba(0,0,0,0.6), inset 0 1px 0 rgba(255,255,255,0.12)'
              : '0 12px 32px rgba(0,0,0,0.5), inset 0 1px 0 rgba(255,255,255,0.08)',
          transform: data.focused
            ? undefined
            : data.canEdit ? 'scale(1.015) translateY(-1px)' : undefined,
        }}
      >
        {/* Thumbnail area - Full height behind info area */}
        <Box
          position="absolute"
          inset={0}
          overflow="hidden"
          borderRadius="8px 8px 0 0"
          flexShrink={0}
          bg="var(--bg-card-solid)"
        >
          {thumbnailUrl ? (
            <Box
              as="img"
              src={thumbnailUrl}
              w="100%"
              h="100%"
              objectFit="contain"
              display="block"
              p={2}
              bg="var(--bg-card-solid)"
              style={{
                filter: 'brightness(0) saturate(100%) invert(35%) sepia(26%) hue-rotate(180deg) brightness(95%) contrast(90%)',
              }}
            />
          ) : (
            <Flex
              w="100%"
              h="100%"
              align="center"
              justify="center"
              background="linear-gradient(90deg, var(--bg-element) 25%, color-mix(in srgb, var(--bg-element), white 5%) 50%, var(--bg-element) 75%)"
              backgroundSize="520px 100%"
              sx={{ animation: !isLoading ? `${shimmer} 1.4s ease infinite` : 'none' }}
            >
              {isLoading && <Spinner size="sm" color="whiteAlpha.400" />}
            </Flex>
          )}
        </Box>

        {/* Info area - name, tech etc */}
        <Flex
          direction="column"
          position="absolute"
          bottom={0}
          left={0}
          right={0}
          h="64px"
          px={3}
          pt={2}
          pb={2}
          bg="color-mix(in srgb, rgba(var(--bg-element-rgb), 0.7), black 20%)"
          backdropFilter="blur(1px) saturate(180%)"
          borderTop="1px solid rgba(255, 255, 255, 0.08)"
          boxShadow="0 -4px 12px rgba(0,0,0,0.15)"
        >
          {/* Name row */}
          <Flex align="flex-start" gap={1.5} flex={1} minH={0}>
            {data.isEditing ? (
              <Input
                ref={inputRef}
                value={data.editName}
                size="sm"
                variant="flushed"
                borderColor="var(--accent)"
                color="gray.100"
                fontWeight="semibold"
                flex={1}
                minW={0}
                fontSize="sm"
                onChange={(e) => data.onEditNameChange(e.target.value)}
                onBlur={data.onEditCommit}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') data.onEditCommit()
                  if (e.key === 'Escape') data.onEditCancel()
                  e.stopPropagation()
                }}
                onClick={(e) => e.stopPropagation()}
              />
            ) : (
              <Text
                flex={1}
                minW={0}
                fontSize="sm"
                fontWeight="semibold"
                color="gray.100"
                noOfLines={1}
                lineHeight={1.35}
                textShadow="0 1px 2px rgba(0,0,0,0.5)"
              >
                {data.name}
              </Text>
            )}

            {!data.isEditing && (
              <Flex align="center" gap={1} onClick={(e) => e.stopPropagation()} mt="-2px">
                <Menu
                  isLazy
                  placement="bottom"
                  closeOnBlur={true}
                  closeOnSelect={true}
                  isOpen={isMenuOpen}
                  onOpen={onMenuOpen}
                  onClose={onMenuClose}
                >
                  <Tooltip
                    label="More actions"
                    placement="top"
                    openDelay={400}
                    isOpen={isTooltipOpen && !isMenuOpen}
                    hasArrow
                  >
                    <MenuButton
                      as={IconButton}
                      aria-label="More actions"
                      icon={
                        <Text fontSize="13px" lineHeight={1} letterSpacing="0.12em">
                          ···
                        </Text>
                      }
                      size="xs"
                      variant="ghost"
                      color="gray.400"
                      _hover={{ color: 'gray.100', bg: 'whiteAlpha.200' }}
                      h="22px"
                      minW="22px"
                      onMouseEnter={() => setIsTooltipOpen(true)}
                      onMouseLeave={() => setIsTooltipOpen(false)}
                    />
                  </Tooltip>
                  <Portal>
                    <MenuList
                      bg="var(--bg-panel)"
                      borderColor="var(--border-main)"
                      minW="152px"
                      py={1}
                      zIndex={8}
                      fontSize="sm"
                      boxShadow="xl"
                      onMouseLeave={onMenuClose}
                    >
                      {data.canEdit && (
                        <MenuItem
                          onClick={(e) => { e.stopPropagation(); data.onStartRename(); onMenuClose() }}
                          bg="transparent"
                          color="gray.300"
                          _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                          _focus={{ bg: 'whiteAlpha.100' }}
                        >
                          Rename
                        </MenuItem>
                      )}
                      <MenuItem
                        onClick={(e) => { e.stopPropagation(); data.onDetails(); onMenuClose() }}
                        bg="transparent"
                        color="gray.300"
                        _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                        _focus={{ bg: 'whiteAlpha.100' }}
                      >
                        Details
                      </MenuItem>
                      {data.canEdit && (
                        <MenuItem
                          onClick={(e) => { e.stopPropagation(); data.onDelete(); onMenuClose() }}
                          bg="transparent"
                          color="red.400"
                          _hover={{ bg: 'whiteAlpha.100', color: 'red.300' }}
                          _focus={{ bg: 'whiteAlpha.100' }}
                        >
                          Delete
                        </MenuItem>
                      )}
                    </MenuList>
                  </Portal>
                </Menu>
              </Flex>
            )}
          </Flex>

          {/* Bottom row: level label + stats */}
          <Flex align="center" justify="space-between" flexShrink={0} gap={1}>
            {data.level_label ? (
              <Text
                fontSize="9px"
                color="var(--accent)"
                textTransform="uppercase"
                letterSpacing="0.1em"
                fontWeight="bold"
                flexShrink={0}
                textShadow="0 1px 2px rgba(0,0,0,0.5)"
              >
                {data.level_label}
              </Text>
            ) : (
              <Box flexShrink={0} />
            )}

            <Text
              fontSize="10px"
              color="gray.400"
              letterSpacing="0.01em"
              flexShrink={0}
              textShadow="0 1px 2px rgba(0,0,0,0.5)"
            >
              {data.counts
                ? `${data.counts.nodes}n · ${data.counts.edges}e`
                : '-'}
            </Text>
          </Flex>
        </Flex>
      </Box>

      {/* WASD navigation hint badge - floats outside the card in the gap between nodes */}
      {data.wasdKey && !data.isEditing && (
        <Box
          position="absolute"
          {...WASD_BADGE_POS[data.wasdKey]}
          zIndex={8}
          pointerEvents="none"
          sx={{ animation: `${badgePop} 0.22s cubic-bezier(0.16, 1, 0.3, 1) both` }}
        >
          <Flex
            align="center"
            justify="center"
            w="30px"
            h="24px"
            bg="var(--bg-menu)"
            border={`1.5px solid ${hexToRgba(accent, 0.65)}`}
            borderRadius="5px"
            boxShadow="0 3px 10px rgba(0,0,0,0.6), inset 0 1px 0 rgba(255,255,255,0.07)"
          >
            <Text
              fontSize="11px"
              fontWeight="800"
              color="#c4dcf5"
              lineHeight={1}
              letterSpacing="0.04em"
            >
              {data.wasdKey.toUpperCase()}
            </Text>
          </Flex>
        </Box>
      )}
    </Box>
  )
}
