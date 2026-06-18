import { Children, isValidElement, useMemo, type ReactElement, type ReactNode } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize, { defaultSchema, type Options as SanitizeSchema } from 'rehype-sanitize'
import remarkGfm from 'remark-gfm'
import { MermaidPreview } from './MermaidPreview'

interface MarkdownPreviewProps {
  markdown: string
  mermaidEnabled?: boolean
}

interface CodeElementProps {
  className?: string
  children?: ReactNode
}

const markdownSanitizeSchema: SanitizeSchema = {
  ...defaultSchema,
  attributes: {
    ...defaultSchema.attributes,
    code: [
      ...(defaultSchema.attributes?.code ?? []),
      ['className', /^language-[\w-]+$/],
    ],
  },
}

function nodeText(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(nodeText).join('')
  if (isValidElement(node)) return nodeText((node as ReactElement<{ children?: ReactNode }>).props.children)
  return ''
}

function codeLanguage(className?: string) {
  return /(?:^|\s)language-([^\s]+)/.exec(className ?? '')?.[1]?.toLowerCase() ?? ''
}

function isCodeElement(node: ReactNode): node is ReactElement<CodeElementProps> {
  return isValidElement(node) && typeof node.props === 'object' && node.props !== null && 'children' in node.props
}

function createMarkdownComponents(mermaidEnabled: boolean): Components {
  return {
    pre({ children }) {
      const child = Children.count(children) === 1 ? Children.only(children) : null
      if (isCodeElement(child)) {
        const language = codeLanguage(child.props.className)
        const rawCode = nodeText(child.props.children).replace(/\n$/, '')

        if (language === 'mermaid' && mermaidEnabled) {
          return <MermaidPreview code={rawCode} />
        }

        return (
          <div className="tld-markdown-codeblock">
            <div className="tld-markdown-codeblock__header">{language || 'text'}</div>
            <pre className="tld-markdown-codeblock__pre">
              <code className={child.props.className}>{child.props.children}</code>
            </pre>
          </div>
        )
      }

      return <pre>{children}</pre>
    },
    code({ className, children, ...props }) {
      return <code className={className} {...props}>{children}</code>
    },
    a({ children, ...props }) {
      return <a {...props} target="_blank" rel="noreferrer">{children}</a>
    },
    input({ ...props }) {
      return <input {...props} readOnly />
    },
  }
}

export function MarkdownPreview({ markdown, mermaidEnabled = true }: MarkdownPreviewProps) {
  const markdownComponents = useMemo(() => createMarkdownComponents(mermaidEnabled), [mermaidEnabled])

  return (
    <div className="tld-markdown-preview" data-testid="markdown-preview">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeRaw, [rehypeSanitize, markdownSanitizeSchema]]}
        components={markdownComponents}
      >
        {markdown}
      </ReactMarkdown>
    </div>
  )
}
