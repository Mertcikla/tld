import { Box, Button, HStack, Input, Spinner, Text, VStack } from '@chakra-ui/react'
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
    <VStack justify="center" align="stretch" spacing={4} h="full" color="whiteAlpha.800" px={6} textAlign="left">
      <Box textAlign="center">
        <Text fontSize="sm" fontWeight="semibold">No notes file for this view</Text>
        <Text fontSize="xs" color="gray.500" mt={1}>Choose where this view's markdown notes should live.</Text>
      </Box>
      <Button
        data-testid="view-markdown-create-private"
        size="sm"
        colorScheme="blue"
        onClick={() => { void onCreateMarkdown?.('PRIVATE_WORKSPACE') }}
        isDisabled={!canEdit || !onCreateMarkdown}
      >
        Create private note
      </Button>
      <VStack spacing={2} align="stretch">
        <Input
          data-testid="view-markdown-repo-path"
          size="sm"
          value={repoPath}
          onChange={(event) => setRepoPath(event.target.value)}
          placeholder={suggestedRepoPath}
        />
        <Button
          data-testid="view-markdown-create-repo"
          size="sm"
          variant="outline"
          onClick={() => { void onCreateMarkdown?.('REPO', repoPath.trim() || suggestedRepoPath) }}
          isDisabled={!canEdit || !onCreateMarkdown}
        >
          Create repo note
        </Button>
      </VStack>
      {showAttachPathInput ? (
        <VStack spacing={2} align="stretch">
          <Input
            data-testid="view-markdown-attach-path"
            size="sm"
            value={attachPath}
            onChange={(event) => setAttachPath(event.target.value)}
            placeholder="docs/overview.md or /absolute/path/overview.md"
          />
          <Button
            data-testid="view-markdown-attach"
            size="sm"
            variant="outline"
            onClick={() => { void onAttachMarkdown?.(attachPath.trim()) }}
            isDisabled={!canEdit || !onAttachMarkdown || !attachPath.trim()}
          >
            Attach existing file
          </Button>
        </VStack>
      ) : (
        <Button
          data-testid="view-markdown-choose-file"
          size="sm"
          variant="outline"
          onClick={() => { void handleAttachPickedFile() }}
          isDisabled={!canEdit || !onAttachMarkdown}
        >
          Attach existing file
        </Button>
      )}
    </VStack>
  )
}
