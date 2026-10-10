import { useCallback, useEffect, useState } from 'react'
import { Box, Button, Flex, IconButton, Spinner, Text, Tooltip } from '@chakra-ui/react'
import { CheckIcon, ChevronLeftIcon, CopyIcon } from '@chakra-ui/icons'
import { api } from '../api/client'
import { copyTextToClipboard } from '../utils/clipboard'
import { toast } from '../utils/toast'
import { MarkdownIcon } from './Icons'
import { MarkdownPreview } from './ViewMarkdownPanel/MarkdownPreview'
import { markdownPanelBodySx } from './ViewMarkdownPanel/styles'

type MermaidPaneState =
  | { status: 'idle'; markdown: string; warnings: string[]; error: '' }
  | { status: 'loading'; markdown: string; warnings: string[]; error: '' }
  | { status: 'ready'; markdown: string; warnings: string[]; error: '' }
  | { status: 'error'; markdown: string; warnings: string[]; error: string }

// The pane stays thin on purpose: the backend renders the same change scene
// the canvas draws, and this only fetches and displays the markdown.
export default function RepositoryChangeMermaid({
  repositoryId,
  comparisonKey,
  plain,
  open,
  collapsed = false,
  overlay = false,
  onExpand,
  onDock,
}: {
  repositoryId: string
  comparisonKey: string
  /** Mirrors the canvas plain toggle so the pane never disagrees with it. */
  plain: boolean
  open: boolean
  /** Rail form: show only the Markdown expand button. */
  collapsed?: boolean
  /** Canvas-covering form: offer a way back to the docked form. */
  overlay?: boolean
  onExpand?: () => void
  onDock?: () => void
}) {
  const [state, setState] = useState<MermaidPaneState>({ status: 'idle', markdown: '', warnings: [], error: '' })
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!open || !repositoryId || !comparisonKey) return undefined
    const controller = new AbortController()
    setState({ status: 'loading', markdown: '', warnings: [], error: '' })
    void api.repositories
      .impactMermaid(repositoryId, comparisonKey, { plain, markdown: true, signal: controller.signal })
      .then((result) => {
        if (!controller.signal.aborted) setState({ status: 'ready', markdown: result.markdown, warnings: result.warnings, error: '' })
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted || (err instanceof Error && err.name === 'AbortError')) return
        setState({ status: 'error', markdown: '', warnings: [], error: err instanceof Error ? err.message : 'Could not load the change diagram' })
      })
    return () => controller.abort()
  }, [open, repositoryId, comparisonKey, plain])

  useEffect(() => {
    setCopied(false)
  }, [state.markdown])

  const handleCopy = useCallback(async () => {
    if (!state.markdown) return
    try {
      await copyTextToClipboard(state.markdown)
      setCopied(true)
      toast({ status: 'success', title: 'Copied change diagram as Markdown' })
    } catch {
      toast({ status: 'error', title: 'Copy failed', description: 'Could not write the change diagram to the clipboard.' })
    }
  }, [state.markdown])

  if (collapsed) {
    return (
      <Flex
        direction="column"
        w="full"
        h="full"
        minW={0}
        minH={0}
        flexShrink={0}
        gap={2}
        align="center"
        justify="center"
        bg="var(--bg-canvas)"
        borderLeft="1px solid"
        borderColor="whiteAlpha.100"
        data-testid="repository-change-mermaid-pane"
      >
        <Tooltip label="Expand change diagram" placement="left" openDelay={200}>
          <IconButton
            size="xs"
            variant="ghost"
            color="var(--accent)"
            aria-label="Expand change diagram"
            icon={<MarkdownIcon size={24} />}
            w="32px"
            h="32px"
            onClick={onExpand}
            data-testid="repository-change-mermaid-expand"
          />
        </Tooltip>
      </Flex>
    )
  }

  return (
    <Flex
      direction="column"
      h="full"
      minW={0}
      minH={0}
      flexShrink={0}
      bg="var(--bg-canvas)"
      borderLeft="1px solid"
      borderColor="whiteAlpha.100"
      data-testid="repository-change-mermaid-pane"
    >
      <Flex
        px={3}
        h="40px"
        flexShrink={0}
        gap={2}
        align="center"
        borderBottom="1px solid"
        borderColor="whiteAlpha.100"
      >
        <Text fontSize="10px" fontWeight="bold" color="gray.500" textTransform="uppercase" letterSpacing="0.06em" whiteSpace="nowrap" minW={0} overflow="hidden" textOverflow="ellipsis">
          Change diagram
        </Text>
        <Box flex={1} />
        {overlay && (
          <Tooltip label="Dock to the canvas edge" placement="bottom" openDelay={200}>
            <IconButton
              size="xs"
              variant="ghost"
              aria-label="Dock change diagram to the canvas edge"
              icon={<ChevronLeftIcon />}
              onClick={onDock}
              data-testid="repository-change-mermaid-dock"
            />
          </Tooltip>
        )}
        <Button
          size="xs"
          variant="ghost"
          color={copied ? 'green.300' : 'gray.300'}
          leftIcon={copied ? <CheckIcon /> : <CopyIcon />}
          isDisabled={!state.markdown}
          onClick={() => void handleCopy()}
          data-testid="repository-change-mermaid-copy"
          whiteSpace="nowrap"
        >
          Copy as Markdown
        </Button>
      </Flex>
      <Box
        flex={1}
        minH={0}
        overflowY="auto"
        sx={markdownPanelBodySx}
        data-testid="repository-change-mermaid"
      >
        {state.status === 'loading' ? (
          <Flex h="full" align="center" justify="center">
            <Spinner size="sm" color="var(--accent)" />
          </Flex>
        ) : state.status === 'error' ? (
          <Text p={4} fontSize="sm" color="red.300">
            {state.error}
          </Text>
        ) : state.markdown ? (
          <Box p={4}>
            {state.warnings.length > 0 && (
              <Text mb={2} fontSize="xs" color="orange.300">
                {state.warnings.join(' ')}
              </Text>
            )}
            <MarkdownPreview markdown={state.markdown} />
          </Box>
        ) : (
          <Text p={4} fontSize="sm" color="gray.500">
            No change diagram to preview yet.
          </Text>
        )}
      </Box>
    </Flex>
  )
}
