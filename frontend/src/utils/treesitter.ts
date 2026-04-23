import Parser from 'web-tree-sitter'

let parserInitialized = false

export async function initTreeSitter() {
  if (parserInitialized) return
  const VERSION = '0.21.0'
  await Parser.init({
    locateFile(scriptName: string) {
      // In newer versions, the WASM file is named tree-sitter.wasm, but some loaders might still ask for web-tree-sitter.wasm
      const name = scriptName === 'web-tree-sitter.wasm' ? 'tree-sitter.wasm' : scriptName
      return `https://unpkg.com/web-tree-sitter@${VERSION}/${name}`
    }
  })
  parserInitialized = true
}

export type SupportedLanguage = 'javascript' | 'typescript' | 'python' | 'java' | 'cpp' | 'go' | 'rust'

const EXT_TO_LANGUAGE: Record<string, SupportedLanguage> = {
  '.js': 'javascript', '.jsx': 'javascript',
  '.ts': 'typescript', '.tsx': 'typescript',
  '.py': 'python', '.go': 'go', '.rs': 'rust',
  '.java': 'java', '.cpp': 'cpp', '.cc': 'cpp', '.cxx': 'cpp', '.c': 'cpp', '.h': 'cpp',
}

export function detectLanguage(path: string): SupportedLanguage | null {
  const parts = path.split('.')
  if (parts.length < 2) return null
  const ext = '.' + parts.pop()!.toLowerCase()
  return EXT_TO_LANGUAGE[ext] ?? null
}

const languageWasms: Record<SupportedLanguage, string> = {
  javascript: 'https://unpkg.com/tree-sitter-wasms@0.1.13/out/tree-sitter-javascript.wasm',
  typescript: 'https://unpkg.com/tree-sitter-wasms@0.1.13/out/tree-sitter-typescript.wasm',
  python: 'https://unpkg.com/tree-sitter-wasms@0.1.13/out/tree-sitter-python.wasm',
  java: 'https://unpkg.com/tree-sitter-wasms@0.1.13/out/tree-sitter-java.wasm',
  cpp: 'https://unpkg.com/tree-sitter-wasms@0.1.13/out/tree-sitter-cpp.wasm',
  go: 'https://unpkg.com/tree-sitter-wasms@0.1.13/out/tree-sitter-go.wasm',
  rust: 'https://unpkg.com/tree-sitter-wasms@0.1.13/out/tree-sitter-rust.wasm',
}

const parsers: Partial<Record<SupportedLanguage, Parser>> = {}

export async function getParser(language: SupportedLanguage): Promise<Parser> {
  await initTreeSitter()

  if (parsers[language]) {
    return parsers[language]!
  }

  const parser = new Parser()
  const langWasmPath = languageWasms[language]
  const Lang = await Parser.Language.load(langWasmPath)
  parser.setLanguage(Lang)
  parsers[language] = parser
  return parser
}

export interface ParsedSymbol {
  name: string
  type: string
  startLine: number
  endLine: number
}

// Extract main symbols using tree-sitter cursor to avoid complex queries across languages.
export function extractSymbols(tree: Parser.Tree, language: SupportedLanguage): ParsedSymbol[] {
  const symbols: ParsedSymbol[] = []

  const isTargetNode = (nodeType: string) => {
    switch (language) {
      case 'javascript':
      case 'typescript':
        return ['class_declaration', 'function_declaration', 'method_definition', 'interface_declaration', 'type_alias_declaration'].includes(nodeType)
      case 'python':
        return ['class_definition', 'function_definition'].includes(nodeType)
      case 'java':
        return ['class_declaration', 'method_declaration', 'interface_declaration'].includes(nodeType)
      case 'cpp':
        return ['class_specifier', 'function_definition', 'struct_specifier'].includes(nodeType)
      case 'go':
        return ['function_declaration', 'method_declaration', 'type_declaration'].includes(nodeType)
      case 'rust':
        return ['function_item', 'struct_item', 'impl_item', 'trait_item'].includes(nodeType)
      default:
        return false
    }
  }

  const getNameField = (node: Parser.SyntaxNode): string => {
    // Attempt to find an identifier child
    if (node.type === 'method_definition') {
      const nameNode = node.childForFieldName('name')
      if (nameNode) return nameNode.text
    }
    for (let i = 0; i < node.childCount; i++) {
      const child = node.child(i)
      if (child && ['identifier', 'type_identifier', 'name'].includes(child.type)) {
        return child.text
      }
    }
    return node.child(1)?.text || 'unknown'
  }

  const traverse = (node: Parser.SyntaxNode) => {
    if (isTargetNode(node.type)) {
      symbols.push({
        name: getNameField(node),
        type: node.type,
        startLine: node.startPosition.row + 1,
        endLine: node.endPosition.row + 1
      })
    }

    for (let i = 0; i < node.childCount; i++) {
      const child = node.child(i)
      if (child) traverse(child)
    }
  }

  traverse(tree.rootNode)

  return symbols
}

export function findSymbolByName(
  tree: Parser.Tree,
  language: SupportedLanguage,
  name: string,
  type: string
): ParsedSymbol | null {
  const all = extractSymbols(tree, language)
  // Exact match
  let found = all.find(s => s.name === name && s.type === type)
  if (found) return found

  // Partial type match (e.g., 'function_item' contains 'function')
  if (type) {
    found = all.find(s => s.name === name && (s.type.includes(type) || type.includes(s.type)))
    if (found) return found
  }

  // Fallback to just name match
  return all.find(s => s.name === name) ?? null
}
