import { useCallback, useMemo, useState } from 'react'
import { Box, Button, Flex, IconButton, Text, Tooltip } from '@chakra-ui/react'
import { CheckIcon, ChevronLeftIcon, CopyIcon } from '@chakra-ui/icons'
import { copyTextToClipboard } from '../utils/clipboard'
import { toast } from '../utils/toast'
import { sceneMermaid, type SceneMermaidView } from '../utils/sceneMermaid'
import type { RepositoryChangeScope } from '../utils/repositoryChangeScene'
import { MarkdownIcon } from './Icons'
import { MarkdownPreview } from './ViewMarkdownPanel/MarkdownPreview'
import { markdownPanelBodySx } from './ViewMarkdownPanel/styles'

export default function RepositoryChangeMermaid({
  repositoryId,
  comparisonKey,
  radius,
  scope,
  view,
  open,
  collapsed = false,
  overlay = false,
  onExpand,
  onDock,
}: {
  repositoryId: string
  comparisonKey: string
  radius: number
  scope: RepositoryChangeScope
  /** The exact filtered view the canvas draws: the diagram mirrors it node for node. */
  view: SceneMermaidView | null
  open: boolean
  /** Rail form: show only the Markdown expand button. */
  collapsed?: boolean
  /** Canvas-covering form: offer a way back to the docked form. */
  overlay?: boolean
  onExpand?: () => void
  onDock?: () => void
}) {
  const [copied, setCopied] = useState(false)

  const markdown = useMemo(() => {
    if (!open || !view) return ''
    const code = sceneMermaid(view, { repositoryId, comparisonKey, scope, radius })
    return `\`\`\`mermaid\n${code}\`\`\`\n`
  }, [open, view, repositoryId, comparisonKey, scope, radius])

  const handleCopy = useCallback(async () => {
    if (!markdown) return
    try {
      await copyTextToClipboard(markdown)
      setCopied(true)
      toast({ status: 'success', title: 'Copied change diagram as Markdown' })
    } catch {
      toast({ status: 'error', title: 'Copy failed', description: 'Could not write the change diagram to the clipboard.' })
    }
  }, [markdown])

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
          isDisabled={!markdown}
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
        {markdown ? (
          <Box p={4}>
            <MarkdownPreview markdown={markdown} />
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