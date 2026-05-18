import {
  Box,
  Button,
  FormLabel,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  Tooltip,
  VStack,
  Wrap,
  WrapItem,
} from '@chakra-ui/react'
import { ACCENT_OPTIONS, BACKGROUND_OPTIONS, ELEMENT_OPTIONS } from '../constants/colors'
import { useTheme } from '../context/ThemeContext'
import { useSourceEditor } from '../utils/sourceEditor'
import { ChevronDownIcon } from '../components/Icons'

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
            minW="140px"
            textAlign="left"
            bg="whiteAlpha.100"
            color="gray.100"
            _hover={{ bg: 'whiteAlpha.200' }}
            _active={{ bg: 'whiteAlpha.300' }}
          >
            {editor === 'zed' ? 'Zed' : 'VS Code'}
          </MenuButton>
          <MenuList>
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
