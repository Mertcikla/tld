import { useEffect, useState } from 'react'
import type { SourceEditor } from '../api/client'

const SOURCE_EDITOR_KEY = 'diag:source-editor'
const DEFAULT_SOURCE_EDITOR: SourceEditor = 'zed'

function readSourceEditor(): SourceEditor {
  if (typeof window === 'undefined') return DEFAULT_SOURCE_EDITOR
  const stored = window.localStorage.getItem(SOURCE_EDITOR_KEY)
  return stored === 'vscode' || stored === 'zed' ? stored : DEFAULT_SOURCE_EDITOR
}

export function getSourceEditor(): SourceEditor {
  return readSourceEditor()
}

export function setSourceEditor(value: SourceEditor) {
  window.localStorage.setItem(SOURCE_EDITOR_KEY, value)
  window.dispatchEvent(new CustomEvent('diag:source-editor-change', { detail: value }))
}

export function useSourceEditor() {
  const [editor, setEditorState] = useState<SourceEditor>(() => readSourceEditor())

  useEffect(() => {
    const handleStorage = (event: StorageEvent) => {
      if (event.key === SOURCE_EDITOR_KEY) {
        setEditorState(readSourceEditor())
      }
    }
    const handleChange = () => setEditorState(readSourceEditor())
    window.addEventListener('storage', handleStorage)
    window.addEventListener('diag:source-editor-change', handleChange)
    return () => {
      window.removeEventListener('storage', handleStorage)
      window.removeEventListener('diag:source-editor-change', handleChange)
    }
  }, [])

  const setEditor = (value: SourceEditor) => {
    setSourceEditor(value)
    setEditorState(value)
  }

  return { editor, setEditor }
}
