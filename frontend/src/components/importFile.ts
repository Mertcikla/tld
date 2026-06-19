export type ImportFileFormat = 'mermaid' | 'structurizr' | 'unsupported-yaml' | 'unsupported'

export function inferImportFileFormat(path: string): ImportFileFormat {
  const clean = path.trim().toLowerCase()
  if (clean.endsWith('.dsl')) return 'structurizr'
  if (clean.endsWith('.yaml') || clean.endsWith('.yml')) return 'unsupported-yaml'
  if (
    clean.endsWith('.md') ||
    clean.endsWith('.txt')
  ) {
    return 'mermaid'
  }
  return 'unsupported'
}

export function unsupportedImportFileMessage(path: string) {
  const format = inferImportFileFormat(path)
  if (format === 'unsupported-yaml') {
    return 'YAML workspace files are not supported by this import flow yet. Use Mermaid or Structurizr DSL.'
  }
  if (format === 'unsupported') {
    return 'Unsupported file type. Choose a Markdown, Structurizr DSL, or text file.'
  }
  return null
}
