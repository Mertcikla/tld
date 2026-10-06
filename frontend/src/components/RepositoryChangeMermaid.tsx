import { useCallback, useEffect, useState } from 'react'
import { Box, Button, Flex, Spinner, Text } from '@chakra-ui/react'
import { CheckIcon, CopyIcon } from '@chakra-ui/icons'
import { api } from '../api/client'
import { copyTextToClipboard } from '../utils/clipboard'
import { toast } from '../utils/toast'
import { MarkdownPreview } from './ViewMarkdownPanel/MarkdownPreview'
import { markdownPanelBodySx } from './ViewMarkdownPanel/styles'

type MermaidPaneState =
  | { status: 'idle'; markdown: string; error: '' }
  | { status: 'loading'; markdown: string; error: '' }
  | { status: 'ready'; markdown: string; error: '' }
  | { status: 'error'; markdown: string; error: string }

export default function RepositoryChangeMermaid({
  repositoryId,
  comparisonKey,
  open,
}: {
  repositoryId: string
  comparisonKey: string
  open: boolean
}) {
  const [state, setState] = useState<MermaidPaneState>({ status: 'idle', markdown: '', error: '' })
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!open || !repositoryId || !comparisonKey) return undefined
    const controller = new AbortController()
    setState({ status: 'loading', markdown: '', error: '' })
    void api.repositories
      .impactMermaid(repositoryId, comparisonKey, { markdown: true, signal: controller.signal })
      .then((result) => {
        if (!controller.signal.aborted) setState({ status: 'ready', markdown: result.markdown, error: '' })
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted || (err instanceof Error && err.name === 'AbortError')) return
        setState({ status: 'error', markdown: '', error: err instanceof Error ? err.message : 'Could not load the change diagram' })
      })
    return () => controller.abort()
  }, [open, repositoryId, comparisonKey])

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
        <Text fontSize="10px" fontWeight="bold" color="gray.500" textTransform="uppercase" letterSpacing="0.06em">
          Change diagram
        </Text>
        <Box flex={1} />
        <Button
          size="xs"
          variant="ghost"
          color={copied ? 'green.300' : 'gray.300'}
          leftIcon={copied ? <CheckIcon /> : <CopyIcon />}
          isDisabled={!state.markdown}
          onClick={() => void handleCopy()}
          data-testid="repository-change-mermaid-copy"
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
