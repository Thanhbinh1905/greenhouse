package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestRunStageAssignmentsUseTimestampHelper(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.AssignStmt:
					for _, lhs := range value.Lhs {
						selector, ok := lhs.(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != "Stage" {
							continue
						}
						receiver, ok := selector.X.(*ast.Ident)
						if ok && receiver.Name == "r" && fn.Name.Name != "setRunStage" {
							t.Errorf("%s assigns r.Stage outside setRunStage", name)
						}
					}
				case *ast.CompositeLit:
					typ, ok := value.Type.(*ast.Ident)
					if !ok || typ.Name != "Run" || fn.Name.Name == "setRunStage" {
						return true
					}
					for _, element := range value.Elts {
						field, ok := element.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						key, ok := field.Key.(*ast.Ident)
						if ok && key.Name == "Stage" {
							t.Errorf("%s initializes Run.Stage outside setRunStage", name)
						}
					}
				}
				return true
			})
		}
	}
}
