import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  FormLabel,
  HStack,
  Input,
  SimpleGrid,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { api } from '../api/client'
import type { ElementReactionSummary, LibraryElement, ThreadResolveEvent, ViewComment, ViewThread } from '../types'
import { useViewEditorContext } from '../pages/ViewEditor/context'

const PREDEFINED_EMOJIS = [
  { emoji: '👍', label: 'Approve' },
  { emoji: '👎', label: 'Disapprove' },
  { emoji: '❤️', label: 'Love' },
  { emoji: '🎉', label: 'Celebrate' },
  { emoji: '🤔', label: 'Thinking' },
  { emoji: '👀', label: 'Reviewing' },
  { emoji: '🚀', label: 'Ship' },
  { emoji: '⚠️', label: 'Warning' },
  { emoji: '✅', label: 'Done' },
]

interface Props {
  element?: LibraryElement | null
  liveThreadUpsert?: ViewThread | null
  liveThreadResolve?: ThreadResolveEvent | null
  liveCommentCreate?: ViewComment | null
  liveReactions?: ElementReactionSummary[]
}

export default function ElementPanelCollaboration({
  element: propsElement,
  liveThreadUpsert,
  liveThreadResolve,
  liveCommentCreate,
  liveReactions,
}: Props) {
  const { viewId, selectedElement } = useViewEditorContext()
  const element = propsElement ?? selectedElement
  const [threads, setThreads] = useState<ViewThread[]>([])
  const [threadInput, setThreadInput] = useState('')
  const [replyInputByThread, setReplyInputByThread] = useState<Record<number, string>>({})
  const [reactions, setReactions] = useState<ElementReactionSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [pickerOpen, setPickerOpen] = useState(false)
  const pickerRef = useRef<HTMLDivElement>(null)

  const refresh = useCallback(async () => {
    if (viewId == null || !element) return
    const [threadList, reactionList] = await Promise.all([
      api.workspace.views.threads.listForElement(viewId, element.id).catch(() => []),
      api.workspace.views.reactions.list(viewId).catch(() => []),
    ])
    setThreads(threadList)
    setReactions(reactionList.filter((item) => item.element_id === element.id))
  }, [element, viewId])

  useEffect(() => {
    if (viewId == null || !element) return
    setLoading(true)
    refresh().finally(() => setLoading(false))
  }, [element, refresh, viewId])

  useEffect(() => {
    if (!element || !liveThreadUpsert || liveThreadUpsert.element_id !== element.id) return
    setThreads((prev) => {
      const index = prev.findIndex((thread) => thread.id === liveThreadUpsert.id)
      if (index === -1) return [...prev, liveThreadUpsert]
      const next = [...prev]
      next[index] = liveThreadUpsert
      return next
    })
  }, [element, liveThreadUpsert])

  useEffect(() => {
    if (!liveThreadResolve) return
    setThreads((prev) => prev.map((thread) =>
      thread.id === liveThreadResolve.thread_id
        ? { ...thread, status: liveThreadResolve.resolved ? 'resolved' : 'open' }
        : thread,
    ))
  }, [liveThreadResolve])

  useEffect(() => {
    if (!liveCommentCreate) return
    setThreads((prev) => prev.map((thread) => {
      if (thread.id !== liveCommentCreate.thread_id) return thread
      if (thread.comments.some((comment) => comment.id === liveCommentCreate.id)) return thread
      return { ...thread, comments: [...thread.comments, liveCommentCreate] }
    }))
  }, [liveCommentCreate])

  useEffect(() => {
    if (!element || !liveReactions) return
    setReactions(liveReactions.filter((item) => item.element_id === element.id))
  }, [element, liveReactions])

  const createThread = async () => {
    if (viewId == null || !element || !threadInput.trim()) return
    setLoading(true)
    try {
      await api.workspace.views.threads.createForElement(viewId, element.id, threadInput.trim())
      setThreadInput('')
      await refresh()
    } finally {
      setLoading(false)
    }
  }

  const reply = async (threadId: number) => {
    if (viewId == null) return
    const value = (replyInputByThread[threadId] || '').trim()
    if (!value) return
    setLoading(true)
    try {
      await api.workspace.views.threads.addComment(viewId, threadId, value)
      setReplyInputByThread((prev) => ({ ...prev, [threadId]: '' }))
      await refresh()
    } finally {
      setLoading(false)
    }
  }

  const resolve = async (threadId: number, resolved: boolean) => {
    if (viewId == null) return
    setLoading(true)
    try {
      await api.workspace.views.threads.resolve(viewId, threadId, resolved)
      await refresh()
    } finally {
      setLoading(false)
    }
  }

  const toggleReaction = async (emoji: string) => {
    if (viewId == null || !element) return
    setLoading(true)
    try {
      await api.workspace.views.reactions.toggleForElement(viewId, element.id, emoji)
      await refresh()
    } finally {
      setLoading(false)
    }
  }

  if (!viewId || !element) return null

  return (
    <VStack align="stretch" spacing={4}>
      <Box borderTop="1px solid" borderColor="whiteAlpha.200" pt={3}>
        <HStack justifyContent="space-between" mb={2}>
          <FormLabel mb={0}>Reactions</FormLabel>
          <Box position="relative" ref={pickerRef}>
            <Button size="xs" variant="outline" onClick={() => setPickerOpen((value) => !value)} isLoading={loading}>
              Add reaction
            </Button>
            {pickerOpen && (
              <>
                <Box position="fixed" inset={0} zIndex={1400} onClick={() => setPickerOpen(false)} />
                <Box position="absolute" bottom="calc(100% + 8px)" right={0} zIndex={8} bg="gray.800" p={3} rounded="md" border="1px solid" borderColor="whiteAlpha.300" shadow="dark-lg" w="216px">
                  <SimpleGrid columns={5} spacing={2}>
                    {PREDEFINED_EMOJIS.map(({ emoji, label }) => (
                      <Tooltip key={emoji} label={label} placement="top" hasArrow>
                        <Box as="button" aria-label={label} w="32px" h="32px" fontSize="18px" rounded="md" _hover={{ bg: 'whiteAlpha.200' }} onClick={() => { setPickerOpen(false); void toggleReaction(emoji) }}>
                          {emoji}
                        </Box>
                      </Tooltip>
                    ))}
                  </SimpleGrid>
                </Box>
              </>
            )}
          </Box>
        </HStack>
        {reactions.length > 0 && (
          <HStack spacing={1} mb={2} flexWrap="wrap">
            {reactions.map((item) => (
              <Box key={item.emoji} as="button" px={2} py="3px" rounded="full" fontSize="13px" border="1px solid" borderColor={item.reacted_by_me ? 'var(--accent)' : 'whiteAlpha.300'} bg={item.reacted_by_me ? 'blue.900' : 'whiteAlpha.50'} onClick={() => toggleReaction(item.emoji)}>
                {item.emoji} {item.count > 1 ? item.count : ''}
              </Box>
            ))}
          </HStack>
        )}
      </Box>

      <Box borderTop="1px solid" borderColor="whiteAlpha.200" pt={3}>
        <FormLabel>Comments & Threads</FormLabel>
        <HStack>
          <Input size="sm" value={threadInput} onChange={(event) => setThreadInput(event.target.value)} placeholder="Add a comment to this element..." onKeyDown={(event) => event.key === 'Enter' && (event.preventDefault(), createThread())} />
          <Button size="sm" colorScheme="blue" onClick={createThread} isLoading={loading}>Post</Button>
        </HStack>
        <VStack mt={3} spacing={3} align="stretch">
          {threads.map((thread) => (
            <Box key={thread.id} border="1px solid" borderColor="whiteAlpha.200" rounded="md" p={2}>
              <HStack justify="space-between" mb={2}>
                <Badge colorScheme={thread.status === 'resolved' ? 'green' : 'blue'}>{thread.status}</Badge>
                <Button size="xs" variant="ghost" onClick={() => resolve(thread.id, thread.status !== 'resolved')}>
                  {thread.status === 'resolved' ? 'Reopen' : 'Resolve'}
                </Button>
              </HStack>
              <VStack align="stretch" spacing={1}>
                {thread.comments.map((comment) => (
                  <Box key={comment.id} bg="whiteAlpha.50" rounded="sm" px={2} py={1}>
                    <Text fontSize="xs" color="var(--accent)">{comment.author_username}</Text>
                    <Text fontSize="sm" color="whiteAlpha.900">{comment.body}</Text>
                  </Box>
                ))}
              </VStack>
              <HStack mt={2}>
                <Input size="sm" value={replyInputByThread[thread.id] || ''} onChange={(event) => setReplyInputByThread((prev) => ({ ...prev, [thread.id]: event.target.value }))} placeholder="Write a reply..." onKeyDown={(event) => event.key === 'Enter' && (event.preventDefault(), reply(thread.id))} />
                <Button size="sm" onClick={() => reply(thread.id)} isLoading={loading}>Reply</Button>
              </HStack>
            </Box>
          ))}
          {!threads.length && !loading && <Text fontSize="sm" color="gray.400">No threads yet.</Text>}
        </VStack>
      </Box>
    </VStack>
  )
}
