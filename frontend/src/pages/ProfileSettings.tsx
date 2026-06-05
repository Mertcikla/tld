import { useEffect, useState } from 'react'
import {
  Box,
  Button,
  FormControl,
  FormLabel,
  Input,
  Text,
  VStack,
  useToast,
} from '@chakra-ui/react'
import { getCollaborationIdentity, saveCollaborationIdentity, type CollaborationIdentity } from '../api/collaborationIdentity'

export default function ProfileSettings() {
  const toast = useToast()
  const [profile, setProfile] = useState<CollaborationIdentity | null>(null)
  const [userId, setUserId] = useState('')
  const [username, setUsername] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const next = getCollaborationIdentity()
    setProfile(next)
    setUserId(next.user_id)
    setUsername(next.username)
  }, [])

  const save = async () => {
    setSaving(true)
    setError(null)
    try {
      const next = saveCollaborationIdentity({ user_id: userId, username })
      setProfile(next)
      setUserId(next.user_id)
      setUsername(next.username)
      toast({ status: 'success', title: 'Profile saved' })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save identity')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Box maxW="520px">
      <VStack align="stretch" spacing={5}>
        <Box>
          <Text fontSize="xl" fontWeight="semibold" color="gray.100">Collaboration</Text>
        </Box>

        <FormControl>
          <FormLabel>User ID</FormLabel>
          <Input value={userId} onChange={(event) => setUserId(event.target.value)} autoComplete="off" />
        </FormControl>

        <FormControl>
          <FormLabel>Username</FormLabel>
          <Input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" />
        </FormControl>

        {error && <Text fontSize="sm" color="red.300">{error}</Text>}

        <Button alignSelf="flex-start" colorScheme="blue" onClick={save} isLoading={saving} isDisabled={!profile}>
          Save
        </Button>
      </VStack>
    </Box>
  )
}
