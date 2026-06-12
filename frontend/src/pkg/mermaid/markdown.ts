export interface MermaidMarkdownBlock {
  start: number
  end: number
  codeStart: number
  codeEnd: number
  fence: string
  code: string
  viewId: number | null
}

export type MermaidMarkdownSyncStatus = 'missing' | 'synced' | 'stale'

function lineWithoutTerminator(line: string) {
  return line.replace(/\r?\n$/, '')
}

function openingFence(line: string) {
  const match = lineWithoutTerminator(line).match(/^[ \t]*(`{3,}|~{3,})[ \t]*([A-Za-z0-9_-]+)?(?:[ \t].*)?$/)
  if (!match) return null
  const language = (match[2] ?? '').toLowerCase()
  if (language !== 'mermaid') return null
  return match[1]
}

function isClosingFence(line: string, fence: string) {
  const trimmed = lineWithoutTerminator(line).trim()
  return trimmed === fence || trimmed.startsWith(fence)
}

function normalizeMermaidCode(value: string) {
  return value.replace(/\r\n/g, '\n').trim()
}

export function mermaidCodeEquals(left: string, right: string) {
  return normalizeMermaidCode(left) === normalizeMermaidCode(right)
}

export function extractTldMermaidViewId(code: string): number | null {
  for (const line of code.split(/\r?\n/)) {
    const body = line.trim().match(/^%%[ \t]+tld\/v1(?:[ \t]+(.*))?$/)?.[1]
    if (body === undefined) continue
    const raw = body.match(/(?:^|\s)(?:view|viewId)=([0-9]+)/)?.[1]
    if (!raw) return null
    const id = Number(raw)
    return Number.isSafeInteger(id) && id > 0 ? id : null
  }
  return null
}

export function findMermaidMarkdownBlocks(markdown: string): MermaidMarkdownBlock[] {
  const blocks: MermaidMarkdownBlock[] = []
  const lines = markdown.match(/.*(?:\r?\n|$)/g) ?? []
  let offset = 0

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]
    if (line === '') break
    const fence = openingFence(line)
    if (!fence) {
      offset += line.length
      continue
    }

    const start = offset
    const codeStart = offset + line.length
    offset += line.length
    let codeEnd = markdown.length
    let end = markdown.length

    for (index += 1; index < lines.length; index += 1) {
      const nextLine = lines[index]
      if (nextLine === '') break
      if (isClosingFence(nextLine, fence)) {
        codeEnd = offset
        end = offset + nextLine.length
        offset = end
        break
      }
      offset += nextLine.length
    }

    const code = markdown.slice(codeStart, codeEnd).replace(/\r?\n$/, '')
    blocks.push({
      start,
      end,
      codeStart,
      codeEnd,
      fence,
      code,
      viewId: extractTldMermaidViewId(code),
    })
  }

  return blocks
}

export function findMermaidMarkdownBlockForView(markdown: string, viewId: number) {
  return findMermaidMarkdownBlocks(markdown).find((block) => block.viewId === viewId) ?? null
}

export function mermaidMarkdownBlock(code: string) {
  return `\`\`\`mermaid\n${code.replace(/\r\n/g, '\n').trim()}\n\`\`\`\n`
}

export function upsertMermaidMarkdownBlock(markdown: string, viewId: number, code: string) {
  const block = findMermaidMarkdownBlockForView(markdown, viewId)
  const nextBlock = mermaidMarkdownBlock(code)
  if (block) {
    return `${markdown.slice(0, block.start)}${nextBlock}${markdown.slice(block.end)}`
  }

  const trimmedEnd = markdown.replace(/\s*$/, '')
  return trimmedEnd ? `${trimmedEnd}\n\n${nextBlock}` : nextBlock
}

export function getMermaidMarkdownSyncStatus(markdown: string, viewId: number, code: string): MermaidMarkdownSyncStatus {
  const block = findMermaidMarkdownBlockForView(markdown, viewId)
  if (!block) return 'missing'
  return mermaidCodeEquals(block.code, code) ? 'synced' : 'stale'
}
