import { ChevronDownIcon } from '@chakra-ui/icons'
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import {
  Box,
  Button,
  HStack,
  Input,
  Menu,
  MenuButton,
  MenuItemOption,
  MenuList,
  MenuOptionGroup,
  Portal,
  Text,
} from '@chakra-ui/react'
import { api } from '../api/client'

type PickerOption = { value: string; label: string }
type PickerGroup = { label?: string; options: PickerOption[] }

export default function RepositoryTargetPicker({
  value,
  repositoryId,
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
  repositoryId?: string
  groups: PickerGroup[]
  onChange: (value: string) => void | boolean
  isDisabled?: boolean
  'aria-label': string
  'data-testid'?: string
  triggerContent?: ReactNode
  compact?: boolean
  card?: boolean
}) {
  const label = groups.flatMap((group) => group.options).find((option) => option.value === value)?.label ?? value
  const [isOpen, setIsOpen] = useState(false)
  const [commitHash, setCommitHash] = useState('')
  const [commitError, setCommitError] = useState('')
  const [resolving, setResolving] = useState(false)
  const request = useRef({ id: 0 }).current
  useEffect(() => {
    setIsOpen(false)
    setCommitHash('')
    setCommitError('')
    setResolving(false)
    return () => { request.id++ }
  }, [repositoryId, isDisabled, request])
  const close = () => {
    request.id++
    setIsOpen(false)
    setResolving(false)
  }
  const submitCommit = async () => {
    if (!repositoryId || isDisabled || resolving) return
    const hash = commitHash.trim().toLowerCase()
    if (!/^[a-f0-9]{4,64}$/i.test(hash)) {
      setCommitError('Enter a full or abbreviated commit hash (at least 4 characters).')
      return
    }
    const currentRequest = ++request.id
    setCommitError('')
    setResolving(true)
    try {
      const { commit } = await api.repositories.commitDetails(repositoryId, hash)
      if (request.id !== currentRequest) return
      if (!commit) throw new Error('Commit not found.')
      if (onChange(`commit:${commit.sha}`) === false) {
        setCommitError('Base must precede Head. Choose another commit.')
        return
      }
      close()
    } catch (err) {
      if (request.id === currentRequest) setCommitError(err instanceof Error ? err.message : 'Could not find this commit.')
    } finally {
      if (request.id === currentRequest) setResolving(false)
    }
  }
  const renderGroups = (items: PickerGroup[]) => (
    <MenuOptionGroup type="radio" value={value} onChange={(next) => onChange(next as string)}>
      {items.filter((group) => group.options.length).flatMap((group, index) => [
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
  )
  return (
    <Menu matchWidth placement="bottom-start" isLazy isOpen={isOpen} onClose={close} onOpen={() => {
      setCommitHash('')
      setCommitError('')
      setIsOpen(true)
    }}>
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
          {renderGroups(groups.slice(0, 1))}
          {card && repositoryId && (
            <Box as="form" px={3} py={2} borderY="1px solid" borderColor="whiteAlpha.100" onSubmit={(event: FormEvent<HTMLFormElement>) => { event.preventDefault(); event.stopPropagation(); void submitCommit() }}>
              <HStack spacing={2}>
                <Input
                  size="xs"
                  minW={0}
                  h="28px"
                  borderRadius="md"
                  fontFamily="mono"
                  fontSize="xs"
                  placeholder="Commit hash…"
                  aria-label={`Commit hash for ${ariaLabel.toLowerCase()}`}
                  value={commitHash}
                  isInvalid={!!commitError}
                  isDisabled={isDisabled || resolving}
                  autoComplete="off"
                  spellCheck={false}
                  onChange={(event) => { setCommitHash(event.target.value); setCommitError('') }}
                  onKeyDown={(event) => { if (event.key !== 'Escape' && event.key !== 'Tab') event.stopPropagation() }}
                />
                <Button type="submit" size="xs" h="28px" flexShrink={0} isLoading={resolving} isDisabled={isDisabled || !commitHash.trim()} aria-label={`Use commit hash for ${ariaLabel.toLowerCase()}`}>Use commit</Button>
              </HStack>
              {commitError && <Text fontSize="xs" color="red.300" mt={1} role="alert">{commitError}</Text>}
            </Box>
          )}
          {renderGroups(groups.slice(1))}
        </MenuList>
      </Portal>
    </Menu>
  )
}
