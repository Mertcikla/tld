import type { CodeBlockEditorDescriptor } from '@mdxeditor/editor'
import { MermaidCollapsedCodeBlock } from './MermaidCollapsedCodeBlock'

export const MERMAID_CODE_BLOCK_DESCRIPTOR: CodeBlockEditorDescriptor = {
  priority: 10,
  match: (language) => (language ?? '').toLowerCase() === 'mermaid',
  Editor: MermaidCollapsedCodeBlock,
}
