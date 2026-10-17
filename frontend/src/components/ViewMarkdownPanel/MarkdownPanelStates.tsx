import { AttachmentIcon, ArrowForwardIcon, EditIcon } from '@chakra-ui/icons'
import { Box, Button, FormControl, FormLabel, HStack, Input, Spinner, Text, VStack } from '@chakra-ui/react'
import { GitIcon, MarkdownIcon } from '../Icons'
import type { ViewMarkdownDocument } from '../../types'

interface LoadingMarkdownStateProps {
  label?: string
}

export function LoadingMarkdownState({ label = 'Loading markdown…' }: LoadingMarkdownStateProps) {
  return (
    <VStack justify="center" align="center" spacing={3} h="full" color="whiteAlpha.700">
      <Spinner size="md" color="blue.300" />
      <Text fontSize="sm">{label}</Text>
    </VStack>
  )
}

interface MissingMarkdownStateProps {
  attachPath: string
  canEdit: boolean
  handleAttachPickedFile: () => Promise<void> | void
  markdown: ViewMarkdownDocument
  onAttachMarkdown?: (path: string) => Promise<void> | void
  onForceSave?: (markdown: string) => Promise<void> | void
  onReload?: () => Promise<void> | void
  onUnlinkMarkdown?: (options?: { deleteManagedFile: boolean }) => Promise<void> | void
  setAttachPath: (path: string) => void
  showAttachPathInput: boolean
  showNativeAttachPicker: boolean
  viewName?: string | null
}

export function MissingMarkdownState({
  attachPath,
  canEdit,
  handleAttachPickedFile,
  markdown,
  onAttachMarkdown,
  onForceSave,
  onReload,
  onUnlinkMarkdown,
  setAttachPath,
  showAttachPathInput,
  showNativeAttachPicker,
  viewName,
}: MissingMarkdownStateProps) {
  return (
    <VStack justify="center" align="center" spacing={3} h="full" color="whiteAlpha.800" px={6} textAlign="center">
      <Text fontSize="sm" fontWeight="semibold">Markdown file is missing</Text>
      <Text fontSize="xs" color="gray.500" maxW="320px" noOfLines={2}>{markdown.path}</Text>
      <HStack spacing={2} flexWrap="wrap" justify="center">
        <Button size="sm" variant="outline" onClick={() => { void onReload?.() }}>Reload</Button>
        <Button
          data-testid="view-markdown-create-replacement"
          size="sm"
          colorScheme="blue"
          onClick={() => { void onForceSave?.(viewName?.trim() ? `# ${viewName.trim()}\n\n` : '') }}
          isDisabled={!canEdit || !onForceSave}
        >
          Create replacement
        </Button>
        <Button
          data-testid="view-markdown-missing-detach"
          size="sm"
          variant="outline"
          colorScheme="red"
          onClick={() => { void onUnlinkMarkdown?.({ deleteManagedFile: false }) }}
          isDisabled={!canEdit || !onUnlinkMarkdown}
        >
          Detach
        </Button>
      </HStack>
      {canEdit && onAttachMarkdown && (
        <VStack spacing={2} w="full" maxW="360px" pt={2}>
          {showAttachPathInput && (
            <Input
              data-testid="view-markdown-attach-path"
              size="sm"
              value={attachPath}
              onChange={(event) => setAttachPath(event.target.value)}
              placeholder="docs/overview.md or /absolute/path/overview.md"
            />
          )}
          <HStack spacing={2} w="full">
            {showAttachPathInput && (
              <Button data-testid="view-markdown-relink" size="sm" variant="outline" flex={1} onClick={() => { void onAttachMarkdown(attachPath.trim()) }} isDisabled={!attachPath.trim()}>
                Relink
              </Button>
            )}
            {showNativeAttachPicker && (
              <Button data-testid="view-markdown-choose-file" size="sm" variant="outline" flex={1} onClick={() => { void handleAttachPickedFile() }}>
                Choose file
              </Button>
            )}
          </HStack>
        </VStack>
      )}
    </VStack>
  )
}

interface MarkdownSetupStateProps {
  attachPath: string
  canEdit: boolean
  dbOnlyNotes?: boolean
  handleAttachPickedFile: () => Promise<void> | void
  onAttachMarkdown?: (path: string) => Promise<void> | void
  onCreateMarkdown?: (targetKind: string, path?: string) => Promise<void> | void
  repoPath: string
  setAttachPath: (path: string) => void
  setRepoPath: (path: string) => void
  showAttachPathInput: boolean
  suggestedRepoPath: string
}

export function MarkdownSetupState({
  attachPath,
  canEdit,
  dbOnlyNotes = false,
  handleAttachPickedFile,
  onAttachMarkdown,
  onCreateMarkdown,
  repoPath,
  setAttachPath,
  setRepoPath,
  showAttachPathInput,
  suggestedRepoPath,
}: MarkdownSetupStateProps) {
  return (
    <Box h="full" overflowY="auto" px={6} py={10} color="gray.100">
      <VStack align="stretch" spacing={6} w="full" maxW="400px" mx="auto">
        <Box>
          <HStack spacing={2} color="var(--accent)" mb={6}>
            <MarkdownIcon size={22} />
            <Text fontSize="xs" fontWeight="semibold" letterSpacing="0.08em" textTransform="uppercase">
              Markdown notes
            </Text>
          </HStack>
          <Text as="h2" fontSize="24px" fontWeight="semibold" letterSpacing="-0.025em" lineHeight="short" mb={3}>
            Choose your workflow
          </Text>
        </Box>

        <Box
          borderWidth="1px"
          borderColor="rgba(var(--accent-rgb), 0.3)"
          borderRadius="xl"
          bg="rgba(var(--accent-rgb), 0.05)"
          p={5}
        >
          <HStack spacing={3} mb={3}>
            <Box display="flex" alignItems="center" justifyContent="center" boxSize={9} borderRadius="lg" bg="rgba(var(--accent-rgb), 0.1)" color="var(--accent)" flexShrink={0}>
              <EditIcon boxSize={4} />
            </Box>
            <Text fontSize="sm" fontWeight="semibold">{dbOnlyNotes ? 'Start a note' : 'Local note'}</Text>
          </HStack>
          <Text fontSize="sm" color="gray.400" lineHeight="tall" mb={4}>
            {dbOnlyNotes
              ? 'A place for notes attached to this view. Start writing and export whenever you need.'
              : 'Just for your workspace. Keep personal notes without adding a file to your repository.'}
          </Text>
          <Button
            data-testid="view-markdown-create-private"
            size="sm"
            w="full"
            h={9}
            bg="var(--accent)"
            color="gray.900"
            boxShadow="none"
            _hover={{ bg: 'var(--accent)', filter: 'brightness(1.1)' }}
            rightIcon={<ArrowForwardIcon />}
            onClick={() => { void onCreateMarkdown?.(dbOnlyNotes ? 'PRIVATE_APP' : 'PRIVATE_WORKSPACE') }}
            isDisabled={!canEdit || !onCreateMarkdown}
          >
            {dbOnlyNotes ? 'Create note' : 'Create local note'}
          </Button>
        </Box>

        {!dbOnlyNotes && (
          <VStack align="stretch" spacing={3}>
            <HStack spacing={3} mb={1}>
              <Box h="1px" flex={1} bg="var(--border-main)" />
              <Text fontSize="xs" color="gray.500">Or use a Markdown file</Text>
              <Box h="1px" flex={1} bg="var(--border-main)" />
            </HStack>

            <Box borderWidth="1px" borderColor="var(--border-main)" borderRadius="xl" p={5}>
              <HStack spacing={2} mb={2}>
                <Box color="gray.400" display="flex"><GitIcon size={16} /></Box>
                <Text fontSize="sm" fontWeight="semibold">File in repository</Text>
              </HStack>
              <Text fontSize="xs" color="gray.400" lineHeight="tall" mb={4}>
                Keep your documentation with your code and share it with your team.
              </Text>
              <FormControl mb={3}>
                <FormLabel fontSize="xs" color="gray.400" mb={2}>File path</FormLabel>
                <Input
                  data-testid="view-markdown-repo-path"
                  size="sm"
                  fontFamily="mono"
                  fontSize="xs"
                  borderRadius="md"
                  value={repoPath}
                  onChange={(event) => setRepoPath(event.target.value)}
                  placeholder={suggestedRepoPath}
                />
              </FormControl>
              <Button
                data-testid="view-markdown-create-repo"
                size="sm"
                w="full"
                variant="outline"
                onClick={() => { void onCreateMarkdown?.('REPO', repoPath.trim() || suggestedRepoPath) }}
                isDisabled={!canEdit || !onCreateMarkdown}
              >
                Create file
              </Button>
            </Box>

            <Box borderWidth="1px" borderColor="var(--border-main)" borderRadius="xl" p={5}>
              <HStack spacing={2} mb={2}>
                <AttachmentIcon boxSize={4} color="gray.400" />
                <Text fontSize="sm" fontWeight="semibold">Attach an existing file</Text>
              </HStack>
              <Text fontSize="xs" color="gray.400" lineHeight="tall" mb={4}>
                Bring in a Markdown document from your project or elsewhere on disk.
              </Text>
              {showAttachPathInput && (
                <FormControl mb={3}>
                  <FormLabel fontSize="xs" color="gray.400" mb={2}>File path or absolute path</FormLabel>
                  <Input
                    data-testid="view-markdown-attach-path"
                    size="sm"
                    fontFamily="mono"
                    fontSize="xs"
                    borderRadius="md"
                    value={attachPath}
                    onChange={(event) => setAttachPath(event.target.value)}
                    placeholder="docs/overview.md"
                  />
                </FormControl>
              )}
              {showAttachPathInput ? (
                <Button
                  data-testid="view-markdown-attach"
                  size="sm"
                  w="full"
                  variant="outline"
                  onClick={() => { void onAttachMarkdown?.(attachPath.trim()) }}
                  isDisabled={!canEdit || !onAttachMarkdown || !attachPath.trim()}
                >
                  Attach file
                </Button>
              ) : (
                <Button
                  data-testid="view-markdown-choose-file"
                  size="sm"
                  w="full"
                  variant="outline"
                  onClick={() => { void handleAttachPickedFile() }}
                  isDisabled={!canEdit || !onAttachMarkdown}
                >
                  Choose file…
                </Button>
              )}
            </Box>
          </VStack>
        )}

      </VStack>
    </Box>
  )
}
