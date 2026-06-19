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
  qualifiedName?: string
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
        return ['abstract_class_declaration', 'class_declaration', 'enum_declaration', 'function_declaration', 'generator_function_declaration', 'interface_declaration', 'method_definition', 'type_alias_declaration', 'variable_declarator'].includes(nodeType)
      case 'python':
        return ['class_definition', 'function_definition'].includes(nodeType)
      case 'java':
        return ['class_declaration', 'constructor_declaration', 'enum_declaration', 'interface_declaration', 'method_declaration', 'record_declaration'].includes(nodeType)
      case 'cpp':
        return ['class_specifier', 'declaration', 'enum_specifier', 'function_definition', 'struct_specifier'].includes(nodeType)
      case 'go':
        return ['function_declaration', 'method_declaration', 'type_alias', 'type_declaration', 'type_spec'].includes(nodeType)
      case 'rust':
        return ['enum_item', 'function_item', 'function_signature_item', 'mod_item', 'struct_item', 'trait_item', 'type_item'].includes(nodeType)
      default:
        return false
    }
  }

  const shouldIncludeNode = (node: Parser.SyntaxNode) => {
    if (!isTargetNode(node.type)) return false
    if (node.type === 'variable_declarator') {
      const valueNode = node.childForFieldName('value')
      return !!valueNode && ['arrow_function', 'function_expression', 'generator_function_expression'].includes(valueNode.type)
    }
    if (language === 'cpp' && node.type === 'declaration') {
      return /\b[A-Za-z_~][A-Za-z0-9_:~]*\s*\(/.test(node.text)
    }
    return true
  }

  const firstIdentifierChild = (node: Parser.SyntaxNode): string => {
    for (let i = 0; i < node.childCount; i++) {
      const child = node.child(i)
      if (child && ['field_identifier', 'identifier', 'property_identifier', 'type_identifier', 'name'].includes(child.type)) {
        return child.text
      }
    }
    return ''
  }

  const simpleCppName = (text: string) => {
    const beforeCall = text.split('(')[0] ?? text
    const parts = beforeCall.trim().split(/::|\s+/).filter(Boolean)
    return parts[parts.length - 1]?.replace(/^~/, '') ?? ''
  }

  const cppOwner = (text: string) => {
    const beforeCall = text.split('(')[0] ?? text
    const parts = beforeCall.trim().split('::')
    if (parts.length < 2) return ''
    return simpleCppName(parts.slice(0, -1).join('::'))
  }

  const goReceiverName = (node: Parser.SyntaxNode) => {
    const receiver = node.childForFieldName('receiver')?.text ?? ''
    const cleaned = receiver.replace(/[()]/g, ' ').replace(/\*/g, ' ').trim()
    const parts = cleaned.split(/\s+/).filter(Boolean)
    return parts[parts.length - 1] ?? ''
  }

  const rustImplTarget = (node: Parser.SyntaxNode) => {
    const typeNode = node.childForFieldName('type')
    if (typeNode) return typeNode.text
    for (let i = 0; i < node.childCount; i++) {
      const child = node.child(i)
      if (child && child.type === 'type_identifier') return child.text
    }
    return ''
  }

  const getNameField = (node: Parser.SyntaxNode): string => {
    const nameNode = node.childForFieldName('name')
    if (nameNode) return nameNode.text
    if (language === 'cpp' && (node.type === 'function_definition' || node.type === 'declaration')) {
      const declarator = node.childForFieldName('declarator')?.text ?? node.text
      return simpleCppName(declarator)
    }
    return firstIdentifierChild(node) || node.child(1)?.text || 'unknown'
  }

  const parentNameForNode = (node: Parser.SyntaxNode, symbolName: string) => {
    switch (node.type) {
      case 'abstract_class_declaration':
      case 'class_declaration':
      case 'class_definition':
      case 'class_specifier':
      case 'enum_declaration':
      case 'enum_item':
      case 'enum_specifier':
      case 'interface_declaration':
      case 'mod_item':
      case 'record_declaration':
      case 'struct_item':
      case 'struct_specifier':
      case 'trait_item':
      case 'type_spec':
        return symbolName
      case 'impl_item':
        return rustImplTarget(node)
      default:
        return ''
    }
  }

  const qualifiedNameForNode = (node: Parser.SyntaxNode, name: string, parentName: string) => {
    if (language === 'go' && node.type === 'method_declaration') {
      const receiver = goReceiverName(node)
      return receiver ? `${receiver}.${name}` : name
    }
    if (language === 'cpp' && (node.type === 'function_definition' || node.type === 'declaration')) {
      const declarator = node.childForFieldName('declarator')?.text ?? node.text
      const owner = cppOwner(declarator) || parentName
      return owner ? `${owner}.${name}` : name
    }
    if (parentName && ['constructor_declaration', 'function_definition', 'function_item', 'function_signature_item', 'method_declaration', 'method_definition'].includes(node.type)) {
      return `${parentName}.${name}`
    }
    return name
  }

  const traverse = (node: Parser.SyntaxNode, parentName = '') => {
    let nextParent = parentName
    if (shouldIncludeNode(node)) {
      const name = getNameField(node)
      const qualifiedName = qualifiedNameForNode(node, name, parentName)
      symbols.push({
        name,
        qualifiedName,
        type: node.type,
        startLine: node.startPosition.row + 1,
        endLine: node.endPosition.row + 1
      })
      nextParent = parentNameForNode(node, name) || nextParent
    } else if (node.type === 'impl_item') {
      nextParent = rustImplTarget(node) || nextParent
    } else {
      const nodeName = getNameField(node)
      nextParent = parentNameForNode(node, nodeName) || nextParent
    }

    for (let i = 0; i < node.childCount; i++) {
      const child = node.child(i)
      if (child) traverse(child, nextParent)
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
  const matchesName = (symbol: ParsedSymbol) => symbol.name === name || symbol.qualifiedName === name

  let found = all.find(s => matchesName(s) && s.type === type)
  if (found) return found

  if (type) {
    found = all.find(s => matchesName(s) && (s.type.includes(type) || type.includes(s.type)))
    if (found) return found
  }

  return all.find(matchesName) ?? null
}
