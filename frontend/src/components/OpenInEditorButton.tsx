import { useEffect, useState } from 'react'
import { Button, Tooltip } from '@chakra-ui/react'
import { ExternalLinkIcon } from '@chakra-ui/icons'
import { api } from '../api/client'
import { useSourceEditor } from '../utils/sourceEditor'
import { toast } from '../utils/toast'

// githubFileUrl builds a public GitHub URL for a file when the element's repo is
// a GitHub remote. Without a branch the repository root is returned.
function githubFileUrl(repo: string, branch: string | null | undefined, filePath: string, line?: number | null): string | null {
  const match = repo.trim().match(/^https?:\/\/(?:www\.)?github\.com\/([^/]+)\/([^/]+?)(?:\.git)?\/?$/i)
  if (!match) return null
  const root = `https://github.com/${match[1]}/${match[2]}`
  const cleanBranch = (branch ?? '').trim()
  const cleanPath = filePath.replace(/^\/+/, '')
  if (!cleanBranch || !cleanPath) return root
  const encoded = cleanPath.split('/').map(encodeURIComponent).join('/')
  const blob = `${root}/blob/${encodeURIComponent(cleanBranch)}/${encoded}`
  return line && line > 0 ? `${blob}#L${line}` : blob
}

export default function OpenInEditorButton({ repo, repositoryId, filePath, line, branch, borderless = false }: {
  repo: string; repositoryId?: string | null; filePath: string; line?: number | null; branch?: string | null; borderless?: boolean
}) {
  const [openingEditor, setOpeningEditor] = useState(false)
  const [editorAvailable, setEditorAvailable] = useState(true)
  const { editor: sourceEditor } = useSourceEditor()

  useEffect(() => {
    let cancelled = false
    void api.system.capabilities().then((caps) => {
      if (!cancelled) setEditorAvailable(caps.editor)
    })
    return () => { cancelled = true }
  }, [])

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

  const buttonProps = {
    leftIcon: <ExternalLinkIcon w="12px" h="12px" />,
    size: 'xs' as const,
    variant: borderless ? ('ghost' as const) : ('outline' as const),
    color: 'whiteAlpha.700',
    borderColor: borderless ? 'transparent' : 'whiteAlpha.200',
    h: '24px',
    fontSize: '11px',
    fontWeight: '600',
    bg: borderless ? 'transparent' : 'whiteAlpha.50',
    _hover: {
      color: 'white',
      bg: 'whiteAlpha.100',
      borderColor: borderless ? 'transparent' : 'whiteAlpha.400',
      textDecoration: 'none',
      transform: 'translateY(-0.5px)',
      boxShadow: borderless ? 'none' : '0 2px 4px rgba(0,0,0,0.2)',
    },
    _active: { bg: 'whiteAlpha.200', transform: 'translateY(0)' },
    transition: 'all 0.1s',
  }

  // Self-hosted servers cannot open the caller's editor. Fall back to the public
  // remote when one is configured, otherwise disable the action.
  if (!editorAvailable) {
    const target = githubFileUrl(repo, branch, filePath, line)
    if (target) {
      return (
        <Tooltip label="Open on GitHub" placement="bottom">
          <Button as="a" href={target} target="_blank" rel="noreferrer" aria-label="Open on GitHub" {...buttonProps}>
            GitHub
          </Button>
        </Tooltip>
      )
    }
    return (
      <Tooltip label="Open in editor is unavailable for this deployment" placement="bottom">
        <Button aria-label="Open in editor unavailable" isDisabled {...buttonProps}>
          Open
        </Button>
      </Tooltip>
    )
  }

  return (
    <Tooltip label={`Open in ${sourceEditor === 'zed' ? 'Zed' : 'VS Code'}`} placement="bottom">
      <Button
        aria-label={`Open in ${sourceEditor === 'zed' ? 'Zed' : 'VS Code'}`}
        isLoading={openingEditor}
        onClick={handleOpenInEditor}
        {...buttonProps}
      >
        Open
      </Button>
    </Tooltip>
  )
}
