import { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  FormLabel,
  HStack,
  Input,
  Text,
  VStack,
} from '@chakra-ui/react'
import { api } from '../api/client'
import type { Connector, ThreadResolveEvent, ViewComment, ViewThread } from '../types'
import { useViewEditorContext } from '../pages/ViewEditor/context'

interface Props {
  connector?: Connector | null
  liveThreadUpsert?: ViewThread | null
  liveThreadResolve?: ThreadResolveEvent | null
  liveCommentCreate?: ViewComment | null
}

export default function ConnectorPanelCollaboration({
  connector: propsConnector,
  liveThreadUpsert,
  liveThreadResolve,
  liveCommentCreate,
}: Props) {
  const { viewId, selectedConnector } = useViewEditorContext()
  const connector = propsConnector ?? selectedConnector
  const [threads, setThreads] = useState<ViewThread[]>([])
  const [threadInput, setThreadInput] = useState('')
  const [replyInputByThread, setReplyInputByThread] = useState<Record<number, string>>({})
  const [loading, setLoading] = useState(false)

  const refresh = useCallback(async () => {
    if (!connector || viewId == null) return
    const list = await api.workspace.views.threads.listForConnector(viewId, connector.id).catch(() => [])
    setThreads(list)
  }, [connector, viewId])

  useEffect(() => {
    if (!connector || viewId == null) return
    setLoading(true)
    refresh().finally(() => setLoading(false))
  }, [connector, refresh, viewId])

  useEffect(() => {
    if (!connector || !liveThreadUpsert || liveThreadUpsert.connector_id !== connector.id) return
    setThreads((prev) => {
      const index = prev.findIndex((thread) => thread.id === liveThreadUpsert.id)
      if (index === -1) return [...prev, liveThreadUpsert]
      const next = [...prev]
      next[index] = liveThreadUpsert
      return next
    })
  }, [connector, liveThreadUpsert])

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

  const createThread = async () => {
    if (!connector || !threadInput.trim() || viewId == null) return
    setLoading(true)
    try {
      await api.workspace.views.threads.createForConnector(viewId, connector.id, threadInput.trim())
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

  if (!viewId || !connector) return null

  return (
    <Box borderTop="1px solid" borderColor="whiteAlpha.200" pt={3}>
      <FormLabel>Comments & Threads</FormLabel>
      <HStack>
        <Input size="sm" value={threadInput} onChange={(event) => setThreadInput(event.target.value)} placeholder="Add a comment to this connector..." onKeyDown={(event) => event.key === 'Enter' && (event.preventDefault(), createThread())} />
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
  )
}
