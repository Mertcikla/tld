import { describe, it, expect } from 'vitest'
import { parseMermaid } from './mermaid'

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
})
