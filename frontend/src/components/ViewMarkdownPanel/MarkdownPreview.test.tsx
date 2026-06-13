import React from 'react'
import { create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import { MarkdownPreview } from './MarkdownPreview'

vi.mock('./MermaidPreview', async () => {
  const ReactModule = await import('react')
  return {
    MermaidPreview: ({ code }: { code: string }) => ReactModule.createElement('div', {
      'data-testid': 'mock-mermaid-preview',
    }, code),
  }
})

describe('MarkdownPreview', () => {
  it('renders GFM tables, task lists, links, and images', () => {
    const renderer = create(
      <MarkdownPreview markdown={[
        '| A | B |',
        '| --- | --- |',
        '| one | two |',
        '',
        '- [x] done',
        '',
        '[site](https://example.com)',
        '',
        '![alt text](https://example.com/image.png)',
      ].join('\n')} />,
    )

    expect(renderer.root.findByType('table')).toBeTruthy()
    expect(renderer.root.findByType('input').props.readOnly).toBe(true)
    expect(renderer.root.findByType('a').props.href).toBe('https://example.com')
    expect(renderer.root.findByType('img').props.alt).toBe('alt text')
  })

  it('sanitizes raw HTML while preserving allowed README-style elements', () => {
    const renderer = create(
      <MarkdownPreview markdown={'<details open><summary>More</summary><script>alert(1)</script><a href="https://example.com" onclick="alert(1)">link</a></details>'} />,
    )

    expect(renderer.root.findByType('details')).toBeTruthy()
    expect(renderer.root.findAllByType('script')).toHaveLength(0)
    expect(renderer.root.findByType('a').props.onClick).toBeUndefined()
  })

  it('renders Mermaid fences through the Mermaid preview component', () => {
    const renderer = create(
      <MarkdownPreview markdown={'```mermaid\nflowchart TD\n  A --> B\n```'} />,
    )

    expect(renderer.root.findByProps({ 'data-testid': 'mock-mermaid-preview' }).children.join('')).toContain('flowchart TD')
  })

  it('renders generic fenced code blocks with a language header', () => {
    const renderer = create(
      <MarkdownPreview markdown={'```go\nfmt.Println("hi")\n```'} />,
    )

    expect(renderer.root.findByProps({ className: 'tld-markdown-codeblock__header' }).children).toEqual(['go'])
  })
})
