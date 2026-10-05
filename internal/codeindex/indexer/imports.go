package indexer

import (
	"strings"
)

// fileImports returns the import paths declared by a source file in order.
// Only Go imports, TypeScript/TSX import and re-export statements, and
// CommonJS require calls are captured.
func fileImports(root *tsNode, src []byte) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw []byte) {
		path := strings.Trim(string(raw), "\"'`")
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	var walk func(*tsNode)
	walk = func(n *tsNode) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case "import_spec":
			if p := n.ChildByFieldName("path"); p != nil {
				add(src[p.StartByte():p.EndByte()])
			}
		case "import_statement", "export_statement":
			if p := n.ChildByFieldName("source"); p != nil {
				add(src[p.StartByte():p.EndByte()])
			}
			// Python `import a, b as c` exposes dotted names as children.
			for i := 0; i < n.NamedChildCount(); i++ {
				child := n.NamedChild(i)
				switch child.Kind() {
				case "dotted_name":
					add(src[child.StartByte():child.EndByte()])
				case "aliased_import":
					if name := child.ChildByFieldName("name"); name != nil {
						add(src[name.StartByte():name.EndByte()])
					}
				}
			}
		case "import_from_statement":
			if module := n.ChildByFieldName("module_name"); module != nil {
				add(src[module.StartByte():module.EndByte()])
			}
		case "call_expression":
			if fn := n.ChildByFieldName("function"); fn != nil && string(src[fn.StartByte():fn.EndByte()]) == "require" {
				if args := n.ChildByFieldName("arguments"); args != nil {
					for i := 0; i < args.NamedChildCount(); i++ {
						c := args.NamedChild(i)
						if c.Kind() == "string" {
							add(src[c.StartByte():c.EndByte()])
							break
						}
					}
				}
			}
		}
		for i := 0; i < n.NamedChildCount(); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(root)
	return out
}
