import { useEffect, useState } from 'react'
import { Box, Button, HStack, Spinner, Text, VStack } from '@chakra-ui/react'
import { faFolder } from '@fortawesome/free-solid-svg-icons'
import { api, type RepositoryFolders } from '../api/client'
import FontAwesomeIcon from './FontAwesomeIcon'

interface RepositoryFolderPickerProps {
  initialPath: string
  onSelect: (path: string) => void
  onCancel: () => void
}

export default function RepositoryFolderPicker({ initialPath, onSelect, onCancel }: RepositoryFolderPickerProps) {
  const [path, setPath] = useState(initialPath)
  const [folders, setFolders] = useState<RepositoryFolders | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    setFolders(null)
    setError('')
    void api.repositories.browseFolders(path, controller.signal).then((result) => {
      if (!controller.signal.aborted) setFolders(result)
    }).catch((err: unknown) => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not browse folders')
    })
    return () => controller.abort()
  }, [path])

  return (
    <Box mt={3} p={3} minW={0} border="1px solid" borderColor="var(--border-main)" borderRadius="md" data-testid="repositories-folder-picker">
      <Text fontSize="xs" color="gray.400" mb={2}>Choose a folder on the machine running tlDiagram.</Text>
      <HStack mb={2} minW={0}>
        <Button size="xs" variant="ghost" onClick={() => setPath('')} isDisabled={path === ''}>Home</Button>
        <Button size="xs" variant="ghost" onClick={() => { if (folders?.parentPath) setPath(folders.parentPath) }} isDisabled={!folders?.parentPath} data-testid="repositories-folder-up">Up</Button>
        <Text fontSize="xs" minW={0} isTruncated title={folders?.path}>{folders?.path}</Text>
      </HStack>
      {error ? <Text fontSize="xs" color="red.300" role="alert">{error}</Text> : !folders ? (
        <HStack role="status"><Spinner size="xs" /><Text fontSize="xs">Loading folders…</Text></HStack>
      ) : (
        <VStack align="stretch" spacing={0} maxH="200px" overflowY="auto" minW={0}>
          {folders.folders.length === 0 && <Text fontSize="xs" color="gray.500">No subfolders.</Text>}
          {folders.folders.map((folder) => (
            <Button
              key={folder}
              size="sm"
              variant="ghost"
              justifyContent="flex-start"
              flexShrink={0}
              leftIcon={<FontAwesomeIcon icon={faFolder} />}
              onClick={() => setPath(`${folders.path.replace(/[/\\]$/, '')}/${folder}`)}
              title={folder}
              aria-label={`Open folder ${folder}`}
            >
              <Text as="span" isTruncated>{folder}</Text>
            </Button>
          ))}
        </VStack>
      )}
      <HStack mt={3} justify="flex-end">
        <Button size="xs" variant="ghost" onClick={onCancel}>Cancel selection</Button>
        <Button size="xs" isDisabled={!folders || !!error} onClick={() => { if (folders) onSelect(folders.path) }} data-testid="repositories-folder-select">Select folder</Button>
      </HStack>
    </Box>
  )
}
