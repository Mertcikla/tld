import { useState } from 'react'
import {
  Box,
  Button,
  FormLabel,
  Input,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  Text,
  Tooltip,
  VStack,
  Wrap,
  WrapItem,
  useToast,
} from '@chakra-ui/react'
import { getCollaborationIdentity, saveCollaborationIdentity } from '../api/collaborationIdentity'
import { ACCENT_OPTIONS, BACKGROUND_OPTIONS, ELEMENT_OPTIONS } from '../constants/colors'
import { useTheme } from '../context/ThemeContext'
import { useSourceEditor } from '../utils/sourceEditor'
import { ChevronDownIcon } from '../components/Icons'

const COMPACT_FIELD_W = '50%'

function CompactUsernameSetting() {
  const toast = useToast()
  const [profile, setProfile] = useState(() => getCollaborationIdentity())
  const [username, setUsername] = useState(profile.username)
  const [error, setError] = useState('')

  const save = () => {
    const nextUsername = username.trim()
    if (nextUsername === profile.username) return

    try {
      const next = saveCollaborationIdentity({ user_id: profile.user_id, username: nextUsername })
      setProfile(next)
      setUsername(next.username)
      setError('')
      toast({ status: 'success', title: 'Username saved' })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save username')
    }
  }

  return (
    <Box w="full">
      <FormLabel mb={2} fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
        Username
      </FormLabel>
      <Input
        size="sm"
        w={COMPACT_FIELD_W}
        maxW="full"
        value={username}
        onChange={(event) => {
          setUsername(event.target.value)
          if (error) setError('')
        }}
        onBlur={save}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.currentTarget.blur()
          }
        }}
        autoComplete="off"
        aria-label="Username"
        textAlign="left"
      />
      {error && <Text mt={2} fontSize="xs" color="red.300">{error}</Text>}
    </Box>
  )
}

export default function AppearanceSettings({ compact = false }: { compact?: boolean }) {
  const { accent, setAccent, background, setBackground, elementColor, setElementColor } = useTheme()
  const { editor, setEditor } = useSourceEditor()
  const swatchSize = compact ? '21px' : '32px'
  const sectionGap = compact ? 5 : 8

  return (
    <VStack align="start" spacing={sectionGap} maxW={compact ? '320px' : '480px'} w="full">


      <Box w="full">
        <FormLabel mb={3} fontSize={compact ? 'xs' : 'sm'} textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
          Accent
        </FormLabel>
        <Wrap spacing={3}>
          {ACCENT_OPTIONS.map((opt) => {
            const isActive = accent === opt.value
            return (
              <WrapItem key={opt.value}>
                <Tooltip label={opt.name} placement="top" openDelay={200}>
                  <Box
                    as="button"
                    w={swatchSize}
                    h={swatchSize}
                    borderRadius="full"
                    bg={opt.value}
                    flexShrink={0}
                    transition="all 0.15s var(--chakra-transitions-easing-pop)"
                    boxShadow={
                      isActive
                        ? `0 0 0 3px ${opt.value}, 0 0 0 5px rgba(0,0,0,0.7), 0 4px 12px rgba(0,0,0,0.4)`
                        : '0 2px 6px rgba(0,0,0,0.4)'
                    }
                    transform={isActive ? 'scale(1.15)' : 'scale(1)'}
                    _hover={{ transform: isActive ? 'scale(1.15)' : 'scale(1.1)' }}
                    onClick={() => setAccent(opt.value)}
                    aria-label={`${opt.name} accent color${isActive ? ' (active)' : ''}`}
                    aria-pressed={isActive}
                  />
                </Tooltip>
              </WrapItem>
            )
          })}
        </Wrap>
      </Box>

      <Box w="full">
        <FormLabel mb={3} fontSize={compact ? 'xs' : 'sm'} textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
          Canvas
        </FormLabel>
        <Wrap spacing={3}>
          {BACKGROUND_OPTIONS.map((opt) => {
            const isActive = background === opt.value
            return (
              <WrapItem key={opt.value}>
                <Tooltip label={opt.name} placement="top" openDelay={200}>
                  <Box
                    as="button"
                    w={swatchSize}
                    h={swatchSize}
                    borderRadius="full"
                    bg={opt.value}
                    flexShrink={0}
                    border="1px solid"
                    borderColor="whiteAlpha.200"
                    transition="all 0.15s var(--chakra-transitions-easing-pop)"
                    boxShadow={
                      isActive
                        ? `0 0 0 3px ${opt.value}, 0 0 0 5px rgba(0,0,0,0.7), 0 4px 12px rgba(0,0,0,0.4)`
                        : '0 2px 6px rgba(0,0,0,0.4)'
                    }
                    transform={isActive ? 'scale(1.15)' : 'scale(1)'}
                    _hover={{ transform: isActive ? 'scale(1.15)' : 'scale(1.1)' }}
                    onClick={() => setBackground(opt.value)}
                    aria-label={`${opt.name} background color${isActive ? ' (active)' : ''}`}
                    aria-pressed={isActive}
                  />
                </Tooltip>
              </WrapItem>
            )
          })}
        </Wrap>
      </Box>

      <Box w="full">
        <FormLabel mb={3} fontSize={compact ? 'xs' : 'sm'} textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
          Elements
        </FormLabel>
        <Wrap spacing={3}>
          {ELEMENT_OPTIONS.map((opt) => {
            const isActive = elementColor === opt.value
            return (
              <WrapItem key={opt.value}>
                <Tooltip label={opt.name} placement="top" openDelay={200}>
                  <Box
                    as="button"
                    w={swatchSize}
                    h={swatchSize}
                    borderRadius="full"
                    bg={opt.value}
                    flexShrink={0}
                    border="1px solid"
                    borderColor="whiteAlpha.200"
                    transition="all 0.15s var(--chakra-transitions-easing-pop)"
                    boxShadow={
                      isActive
                        ? `0 0 0 3px ${opt.value}, 0 0 0 5px rgba(0,0,0,0.7), 0 4px 12px rgba(0,0,0,0.4)`
                        : '0 2px 6px rgba(0,0,0,0.4)'
                    }
                    transform={isActive ? 'scale(1.15)' : 'scale(1)'}
                    _hover={{ transform: isActive ? 'scale(1.15)' : 'scale(1.1)' }}
                    onClick={() => setElementColor(opt.value)}
                    aria-label={`${opt.name} element color${isActive ? ' (active)' : ''}`}
                    aria-pressed={isActive}
                  />
                </Tooltip>
              </WrapItem>
            )
          })}
        </Wrap>
      </Box>

      {compact && <CompactUsernameSetting />}

      <Box w="full">
        <FormLabel mb={3} fontSize={compact ? 'xs' : 'sm'} textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
          Editor
        </FormLabel>
        <Menu>
          <MenuButton
            as={Button}
            size="sm"
            variant="clay"
            rightIcon={<ChevronDownIcon size={12} strokeWidth={4} />}
            w={compact ? COMPACT_FIELD_W : undefined}
            maxW={compact ? 'full' : undefined}
            minW={compact ? undefined : '140px'}
            justifyContent="space-between"
            textAlign="left"
            bg="whiteAlpha.100"
            color="gray.100"
            _hover={{ bg: 'whiteAlpha.200' }}
            _active={{ bg: 'whiteAlpha.300' }}
          >
            {editor === 'zed' ? 'Zed' : 'VS Code'}
          </MenuButton>
          <MenuList w={compact ? COMPACT_FIELD_W : undefined} minW={compact ? COMPACT_FIELD_W : undefined}>
            <MenuItem onClick={() => setEditor('zed')} fontWeight={editor === 'zed' ? 'bold' : 'normal'}>
              Zed
            </MenuItem>
            <MenuItem onClick={() => setEditor('vscode')} fontWeight={editor === 'vscode' ? 'bold' : 'normal'}>
              VS Code
            </MenuItem>
          </MenuList>
        </Menu>
      </Box>
    </VStack>
  )
}
