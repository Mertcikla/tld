import { describe, it, expect } from 'vitest'
import {
  MERMAID_IMPORT_LIMITS,
  extractMermaidCode,
  mermaidImportSourceLimitError,
  parseMermaid,
  parseMermaidAsync,
  parsedMermaidImportLimitError,
  tryParseMermaid,
} from './mermaid'

const compatibleMermaidFixtures = [
  {
    name: 'flowchart',
    sourceUrl: 'https://mermaid.js.org/syntax/flowchart.html',
    code: 'flowchart LR\n  Browser[Browser] --> API[API]\n  API --> DB[(Database)]',
    elements: 3,
    connectors: 2,
  },
  {
    name: 'C4 context',
    sourceUrl: 'https://mermaid.js.org/syntax/c4.html',
    code: `C4Context
  Person(customer, "Customer", "Uses online banking")
  System(app, "Banking App", "Serves account data")
  System_Ext(email, "Email System", "Sends notifications")
  Rel(customer, app, "Uses")
  Rel(app, email, "Sends email")`,
    elements: 3,
    connectors: 2,
  },
  {
    name: 'sequence',
    sourceUrl: 'https://mermaid.js.org/syntax/sequenceDiagram.html',
    code: 'sequenceDiagram\n  Alice->>John: Hello John\n  John-->>Alice: Great',
    elements: 2,
    connectors: 2,
  },
  {
    name: 'class',
    sourceUrl: 'https://mermaid.js.org/syntax/classDiagram.html',
    code: 'classDiagram\n  Animal <|-- Duck\n  Animal : +int age\n  Duck : +quack()',
    elements: 2,
    connectors: 1,
  },
  {
    name: 'entity relationship',
    sourceUrl: 'https://mermaid.js.org/syntax/entityRelationshipDiagram.html',
    code: 'erDiagram\n  CUSTOMER ||--o{ ORDER : places\n  ORDER ||--|{ LINE_ITEM : contains',
    elements: 3,
    connectors: 2,
  },
  {
    name: 'state',
    sourceUrl: 'https://mermaid.js.org/syntax/stateDiagram.html',
    code: 'stateDiagram-v2\n  [*] --> Still\n  Still --> Moving\n  Moving --> Crash',
    elements: 4,
    connectors: 3,
  },
  {
    name: 'requirement',
    sourceUrl: 'https://mermaid.js.org/syntax/requirementDiagram.html',
    code: `requirementDiagram
  requirement checkout_req {
    id: 1
    text: checkout must be reliable
    risk: high
    verifymethod: test
  }
  element checkout_service {
    type: service
    docref: checkout.md
  }
  checkout_service - satisfies -> checkout_req`,
    elements: 2,
    connectors: 1,
  },
  {
    name: 'sankey',
    sourceUrl: 'https://mermaid.js.org/syntax/sankey.html',
    code: 'sankey-beta\n  API,Worker,10\n  Worker,Database,7',
    elements: 3,
    connectors: 2,
  },
  {
    name: 'pie',
    sourceUrl: 'https://mermaid.js.org/syntax/pie.html',
    code: 'pie title Runtime usage\n  "API" : 386\n  "Worker" : 85',
    elements: 3,
    connectors: 2,
  },
  {
    name: 'git graph',
    sourceUrl: 'https://mermaid.js.org/syntax/gitgraph.html',
    code: 'gitGraph\n  commit id:"start"\n  branch develop\n  checkout develop\n  commit id:"work"\n  checkout main\n  merge develop',
    elements: 5,
    connectors: 4,
  },
  {
    name: 'quadrant',
    sourceUrl: 'https://mermaid.js.org/syntax/quadrantChart.html',
    code: `quadrantChart
  title Campaign reach
  x-axis Low Reach --> High Reach
  y-axis Low Engagement --> High Engagement
  Campaign A: [0.3, 0.6]
  Campaign B: [0.78, 0.34]`,
    elements: 3,
    connectors: 2,
  },
  {
    name: 'mindmap',
    sourceUrl: 'https://mermaid.js.org/syntax/mindmap.html',
    code: 'mindmap\n  root((Architecture))\n    API\n      REST\n    Data\n      PostgreSQL',
    elements: 5,
    connectors: 4,
  },
  {
    name: 'journey',
    sourceUrl: 'https://mermaid.js.org/syntax/userJourney.html',
    code: 'journey\n  title Developer workflow\n  section Code\n    Write tests: 5: Dev\n    Review diff: 4: Dev',
    elements: 4,
    connectors: 3,
  },
  {
    name: 'gantt',
    sourceUrl: 'https://mermaid.js.org/syntax/gantt.html',
    code: 'gantt\n  title Release plan\n  dateFormat YYYY-MM-DD\n  section Build\n  Implement import :a1, 2026-01-01, 3d',
    elements: 3,
    connectors: 2,
  },
  {
    name: 'timeline',
    sourceUrl: 'https://mermaid.js.org/syntax/timeline.html',
    code: 'timeline\n  title Release history\n  2025 : Prototype\n  2026 : Stable release',
    elements: 5,
    connectors: 4,
  },
  {
    name: 'xy chart',
    sourceUrl: 'https://mermaid.js.org/syntax/xyChart.html',
    code: 'xychart-beta\n  title "Requests"\n  x-axis [Jan, Feb]\n  y-axis "Count" 0 --> 100\n  bar [40, 70]',
    elements: 4,
    connectors: 3,
  },
  {
    name: 'architecture',
    sourceUrl: 'https://mermaid.js.org/syntax/architecture.html',
    code: `architecture-beta
  service database1(database)[My Database]
  service server(server)[Server]
  database1:R --> L:server`,
    elements: 2,
    connectors: 1,
  },
] as const

describe('Mermaid Importer Compliance', () => {
  it('should parse a simple left-right graph (Example 1)', () => {
    const code = `
graph LR;
A-->B;
A-->C;
B-->D;
C-->D;
`
    const result = parseMermaid(code)
    expect(result.elements.length).toBeGreaterThanOrEqual(4)
    expect(result.connectors.length).toBe(4)
    expect(result.warnings).toHaveLength(0)
    
    const ids = result.elements.map(o => o.ref)
    expect(ids).toContain('A')
    expect(ids).toContain('B')
    expect(ids).toContain('C')
    expect(ids).toContain('D')
  })

  it('should parse a flowchart with labels (Example 2)', () => {
    const code = `
flowchart LR
    a[Chapter 1] --> b[Chapter 2] --> c[Chapter 3]
    c-->d[Using Ledger]
    c-->e[Using Trezor]
    d-->f[Chapter 4]
    e-->f
`
    const result = parseMermaid(code)
    expect(result.elements.length).toBe(6)
    expect(result.connectors.length).toBe(6)
    expect(result.direction).toBe('LR')
    
    const chapter1 = result.elements.find(o => o.ref === 'a')
    expect(chapter1?.name).toBe('Chapter 1')
  })

  it('should parse a graph with dependency sets (Example 4)', () => {
    const code = `
graph TB
    A & B--> C & D
`
    const result = parseMermaid(code)
    // A->C, A->D, B->C, B->D = 4 edges
    expect(result.connectors.length).toBe(4)
  })

  it('should parse a flowchart with shapes and link variants (Example 6)', () => {
    const code = `
graph LR
    A[Square Rect] -- Link text --> B((Circle))
    A --> C(Round Rect)
    B --> D{Rhombus}
    C --> D
`
    const result = parseMermaid(code)
    expect(result.elements.length).toBe(4)
    expect(result.connectors.length).toBe(4)
    
    const edgeWithText = result.connectors.find(e => e.sourceElementRef === 'A' && e.targetElementRef === 'B')
    expect(edgeWithText?.label).toBe('Link text')
  })

  it('should parse a top-down graph (Example 3)', () => {
    const code = `
graph TD;
A-->B;
A-->C;
B-->D;
C-->D;
`
    const result = parseMermaid(code)
    expect(result.connectors.length).toBe(4)
    expect(result.direction).toBe('TD')
  })

  it('should parse a binary tree (Example 5)', () => {
    const code = `
graph TB
    A((1))-->B((2))
    A-->C((3))
    B-->D((4))
    B-->E((5))
    C-->F((6))
    C-->G((7))
    D-->H((8))
    D-->I((9))
    E-->J((10))
`
    const result = parseMermaid(code)
    expect(result.elements.length).toBe(10)
    expect(result.connectors.length).toBe(9)
  })

  it('should parse a flowchart with subgraphs (Example 11)', () => {
    const code = `
graph TB
    c1-->a2
    subgraph one
    a1-->a2
    end
    subgraph two
    b1-->b2
    end
    subgraph three
    c1-->c2
    end
`
    const result = parseMermaid(code)
    expect(result.elements.length).toBeGreaterThanOrEqual(6)
    expect(result.connectors.length).toBe(4)
  })

  it('should parse a decision tree (Example 14 simplified)', () => {
    const code = `
graph TB
A[Do you think online service learning is right for you?]
B[Do you have time to design a service learning component?]
C[What is the civic or public purpose of your discipline?]
D[Do you have departmental or school support?]
E[Are you willing to be a trailblazer?]
F[What type of service learning to you want to plan?]

A==Yes==>B
A--No-->C
B==Yes==>D
B--No-->E
D==Yes==>F
D--No-->E
E==Yes==>F
E--No-->C
`
    const result = parseMermaid(code)
    const ids = result.elements.map(o => o.ref)
    expect(result.elements.length, `IDs found: ${ids.join(', ')}`).toBe(6)
    expect(result.connectors.length).toBe(8)
  })

  it('extracts Mermaid from raw and fenced markdown', () => {
    expect(extractMermaidCode('flowchart LR\n  A --> B')).toBe('flowchart LR\n  A --> B')
    expect(extractMermaidCode('architecture-beta\n  service a(server)[A]')).toBe('architecture-beta\n  service a(server)[A]')
    expect(extractMermaidCode('Before\n\n```mermaid\nflowchart TB\n  A --> B\n```\nAfter')).toBe('flowchart TB\n  A --> B')
    expect(extractMermaidCode('Before\n\n```mmd\nflowchart LR\n  A --> B\n```')).toBe('flowchart LR\n  A --> B')
    expect(extractMermaidCode('```mermaid   \nflowchart LR\n  A --> B\n```')).toBe('flowchart LR\n  A --> B')
    expect(extractMermaidCode('```mermaid\r\nflowchart LR\n  A --> B\r\n```')).toBe('flowchart LR\n  A --> B')
    expect(extractMermaidCode('```mmd' + '\n '.repeat(5000))).toBeNull()
    expect(extractMermaidCode('not a diagram')).toBeNull()
  })

  it('sanitizes br tags from node labels', () => {
    const code = 'graph TD\n  A["Identify Resources<br/>populate_resource"]\n  B["Line 1<br>Line 2"]'
    const result = parseMermaid(code)
    expect(result.elements.length).toBe(2)
    expect(result.elements[0]?.name).toBe('Identify Resources populate_resource')
    expect(result.elements[1]?.name).toBe('Line 1 Line 2')
  })

  it('sanitizes br tags from connector labels', () => {
    const code = 'graph LR\n  A -->|"Label<br/>split"| B'
    const result = parseMermaid(code)
    expect(result.connectors[0]?.label).toBe('Label split')
  })

  it('strips arbitrary HTML tags from labels', () => {
    const code = 'graph TD\n  A["<b>Bold</b> text"]'
    const result = parseMermaid(code)
    expect(result.elements[0]?.name).toBe('Bold text')
  })

  it('decodes Mermaid-safe quote and ampersand entities in labels', () => {
    const code = `flowchart LR
  A["Worker &quot;A&quot; &amp; friends"]
  A -- "writes &quot;to&quot; &amp; reads" --> B["Database"]`

    const result = parseMermaid(code)

    expect(result.warnings).toHaveLength(0)
    expect(result.elements.find((element) => element.ref === 'A')?.name).toBe('Worker "A" & friends')
    expect(result.connectors[0]?.label).toBe('writes "to" & reads')
  })

  it('reports Mermaid import source and parsed size limits', () => {
    expect(mermaidImportSourceLimitError('flowchart LR\n  A --> B')).toBeNull()
    expect(mermaidImportSourceLimitError('x'.repeat(MERMAID_IMPORT_LIMITS.maxSourceBytes + 1))).toContain('Limit is 250 KiB')
    expect(parsedMermaidImportLimitError({
      elements: Array.from({ length: MERMAID_IMPORT_LIMITS.maxElements + 1 }, (_, index) => ({ ref: `n${index}`, name: `Node ${index}` })),
      connectors: [],
    })).toBe(`Mermaid import has ${MERMAID_IMPORT_LIMITS.maxElements + 1} elements. Limit is ${MERMAID_IMPORT_LIMITS.maxElements}.`)
    expect(parsedMermaidImportLimitError({
      elements: [],
      connectors: Array.from({ length: MERMAID_IMPORT_LIMITS.maxConnectors + 1 }, (_, index) => ({ ref: `c${index}`, sourceElementRef: 'a', targetElementRef: 'b' })),
    })).toBe(`Mermaid import has ${MERMAID_IMPORT_LIMITS.maxConnectors + 1} connectors. Limit is ${MERMAID_IMPORT_LIMITS.maxConnectors}.`)
  })

  it('returns null for non-Mermaid clipboard text', () => {
    expect(tryParseMermaid('ordinary text')).toBeNull()
  })

  it('tolerates regular Mermaid comments and ignores malformed TLD metadata', () => {
    const code = `flowchart LR
%% tld/v1
  A[Start]
%% ordinary Mermaid comment
%% tld-element missing kind=database
%% tld-element A x=not-a-number y=2 unknown=value
  A --> B[End]
%% tld-connector A->B rel=bad\\q
%% tld-connector missing->B rel=unused`

    const parsed = parseMermaid(code)

    expect(parsed.warnings).toHaveLength(0)
    expect(parsed.elements).toHaveLength(2)
    expect(parsed.connectors).toHaveLength(1)
    expect(parsed.elements.find((element) => element.ref === 'A')?.kind).toBe('system')
    expect(parsed.elements.find((element) => element.ref === 'A')?.placements[0]?.positionX).toBeUndefined()
    expect(parsed.connectors[0]?.relationship).toBeUndefined()
  })

  it('converts architecture-beta services, junctions, and side-qualified edges', async () => {
    const code = `architecture-beta
    service left_disk(disk)[Disk]
    service top_disk(disk)[Disk]
    service bottom_disk(disk)[Disk]
    service top_gateway(internet)[Gateway]
    service bottom_gateway(internet)[Gateway]
    junction junctionCenter
    junction junctionRight

    left_disk:R -- L:junctionCenter
    top_disk:B -- T:junctionCenter
    bottom_disk:T -- B:junctionCenter
    junctionCenter:R -- L:junctionRight
    top_gateway:B -- T:junctionRight
    bottom_gateway:T -- B:junctionRight`

    const result = await parseMermaidAsync(code)

    expect(result.warnings).toHaveLength(0)
    expect(result.elements).toHaveLength(7)
    expect(result.connectors).toHaveLength(6)
    expect(result.elements.filter((element) => element.name === 'Disk')).toHaveLength(3)
    expect(result.elements.filter((element) => element.name === 'Gateway')).toHaveLength(2)
    expect(result.elements.find((element) => element.ref === 'junctionCenter')?.name).toBe('junctionCenter')
    expect(result.elements.find((element) => element.ref === 'left_disk')?.kind).toBe('database')
    expect(result.elements.find((element) => element.ref === 'top_gateway')?.kind).toBe('external')

    const firstConnector = result.connectors.find((connector) => connector.sourceElementRef === 'left_disk')
    expect(firstConnector?.targetElementRef).toBe('junctionCenter')
    expect(firstConnector?.direction).toBe('none')
    expect(firstConnector?.sourceHandle).toBe('right')
    expect(firstConnector?.targetHandle).toBe('left')
  })

  it.each(compatibleMermaidFixtures)('converts compatible $name fixture from $sourceUrl', async (fixture) => {
    const result = await parseMermaidAsync(fixture.code)

    expect(result.warnings, fixture.name).toHaveLength(0)
    expect(result.elements.length, fixture.name).toBeGreaterThanOrEqual(fixture.elements)
    expect(result.connectors.length, fixture.name).toBeGreaterThanOrEqual(fixture.connectors)
    expect(result.elements.every((element) => element.placements.some((placement) => placement.parentRef === 'root'))).toBe(true)
  })
})
