import { Box, FormLabel, HStack, Select, Text, Tooltip, VStack, Wrap, WrapItem } from '@chakra-ui/react'
import { ACCENT_OPTIONS, BACKGROUND_OPTIONS, ELEMENT_OPTIONS } from '../constants/colors'
import { useTheme } from '../context/ThemeContext'
import { useSourceEditor } from '../utils/sourceEditor'
import type { SourceEditor } from '../api/client'

export default function AppearanceSettings({ compact = false }: { compact?: boolean }) {
  const { accent, setAccent, background, setBackground, elementColor, setElementColor } = useTheme()
  const { editor, setEditor } = useSourceEditor()
  const swatchSize = compact ? '28px' : '32px'
  const sectionGap = compact ? 5 : 8

  return (
    <VStack align="start" spacing={sectionGap} maxW={compact ? '320px' : '480px'} w="full">
      <Box w="full">
        <HStack justify="space-between" align="end" w="full" mb={compact ? 0 : 1}>
          <Box>
            <Text fontFamily="heading" fontSize={compact ? 'md' : 'lg'} fontWeight="bold" color="gray.100" mb={1}>
              Theme
            </Text>
          </Box>
        </HStack>
      </Box>

      <Box w="full">
        <FormLabel mb={3} fontSize={compact ? 'xs' : 'sm'} textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
          Source Editor
        </FormLabel>
        <Select
          size="sm"
          value={editor}
          onChange={(event) => setEditor(event.target.value as SourceEditor)}
          bg="whiteAlpha.50"
          borderColor="whiteAlpha.200"
          color="gray.100"
          maxW="220px"
          _hover={{ borderColor: 'whiteAlpha.400' }}
          _focus={{ borderColor: 'blue.400', boxShadow: '0 0 0 1px var(--chakra-colors-blue-400)' }}
        >
          <option value="zed">Zed</option>
          <option value="vscode">VS Code</option>
        </Select>
      </Box>

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
    </VStack>
  )
}
