import { useState } from 'react'
import { Button, Tooltip } from '@chakra-ui/react'
import { ExternalLinkIcon } from '@chakra-ui/icons'
import { api } from '../api/client'
import { useSourceEditor } from '../utils/sourceEditor'
import { toast } from '../utils/toast'

export default function OpenInEditorButton({ repo, repositoryId, filePath, line, borderless = false }: {
  repo: string; repositoryId?: string | null; filePath: string; line?: number | null; borderless?: boolean
}) {
  const [openingEditor, setOpeningEditor] = useState(false)
  const { editor: sourceEditor } = useSourceEditor()
  const handleOpenInEditor = async () => {
    if (!filePath) return
    setOpeningEditor(true)
    try {
      await api.editor.open({
        editor: sourceEditor,
        repository_id: repositoryId,
        repo,
        file_path: filePath,
        line,
      })
    } catch (err) {
      toast({
        title: 'Failed to open editor',
        description: err instanceof Error ? err.message : String(err),
        status: 'error',
        duration: 4000,
      })
    } finally {
      setOpeningEditor(false)
    }
  }

  return (
    <Tooltip label={`Open in ${sourceEditor === 'zed' ? 'Zed' : 'VS Code'}`} placement="bottom">
      <Button
        aria-label={`Open in ${sourceEditor === 'zed' ? 'Zed' : 'VS Code'}`}
        leftIcon={<ExternalLinkIcon w="12px" h="12px" />}
        size="xs"
        variant={borderless ? 'ghost' : 'outline'}
        color="whiteAlpha.700"
        borderColor={borderless ? 'transparent' : 'whiteAlpha.200'}
        h="24px"
        fontSize="11px"
        fontWeight="600"
        bg={borderless ? 'transparent' : 'whiteAlpha.50'}
        isLoading={openingEditor}
        onClick={handleOpenInEditor}
        _hover={{
          color: 'white',
          bg: 'whiteAlpha.100',
          borderColor: borderless ? 'transparent' : 'whiteAlpha.400',
          textDecoration: 'none',
          transform: 'translateY(-0.5px)',
          boxShadow: borderless ? 'none' : '0 2px 4px rgba(0,0,0,0.2)'
        }}
        _active={{
          bg: 'whiteAlpha.200',
          transform: 'translateY(0)',
        }}
        transition="all 0.1s"
      >
        Open
      </Button>
    </Tooltip>
  )
}
