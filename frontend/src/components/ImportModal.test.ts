import { describe, expect, it } from 'vitest'
import { inferImportFileFormat, unsupportedImportFileMessage } from './importFile'

describe('ImportModal file helpers', () => {
  it('infers supported import formats from file extensions', () => {
    expect(inferImportFileFormat('/tmp/diagram.mmd')).toBe('unsupported')
    expect(inferImportFileFormat('/tmp/diagram.mermaid')).toBe('unsupported')
    expect(inferImportFileFormat('/tmp/diagram.md')).toBe('mermaid')
    expect(inferImportFileFormat('/tmp/workspace.dsl')).toBe('structurizr')
    expect(inferImportFileFormat('/tmp/notes.txt')).toBe('mermaid')
  })

  it('returns explicit unsupported messages for yaml and unknown files', () => {
    expect(unsupportedImportFileMessage('/tmp/elements.yaml')).toContain('YAML workspace files are not supported')
    expect(unsupportedImportFileMessage('/tmp/diagram.json')).toContain('Unsupported file type')
    expect(unsupportedImportFileMessage('/tmp/diagram.mmd')).toContain('Unsupported file type')
  })
})
