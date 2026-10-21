package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestInstrumentationConventions enforces the rules in docs/observability.md
// that a static check can catch, so new code cannot silently opt out of
// trace correlation or outbound HTTP tracing.
func TestInstrumentationConventions(t *testing.T) {
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (path == "dist" || path == "tools" || path == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, v := range conventionViolations(file) {
			t.Errorf("%s: %s", fset.Position(v.pos), v.msg)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type violation struct {
	pos token.Pos
	msg string
}

// slogWithoutContext are the slog helpers that drop ctx, and with it the trace/span IDs.
var slogWithoutContext = map[string]bool{"Debug": true, "Info": true, "Warn": true, "Error": true, "Log": true}

func conventionViolations(file *ast.File) []violation {
	var out []violation
	imports := map[string]string{}
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = path
		if path == "log" {
			out = append(out, violation{imp.Pos(), `import "log": use log/slog with the *Context variants`})
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			pkg, ok := n.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch imports[pkg.Name] {
			case "log/slog":
				if slogWithoutContext[n.Sel.Name] {
					out = append(out, violation{n.Pos(), "slog." + n.Sel.Name + ": use slog." + n.Sel.Name + "Context(ctx, ...) so logs carry the trace context"})
				}
			case "net/http":
				switch n.Sel.Name {
				case "Get", "Head", "Post", "PostForm", "DefaultClient":
					out = append(out, violation{n.Pos(), "http." + n.Sel.Name + ": use an http.Client with an otelhttp.NewTransport"})
				case "NewRequest":
					out = append(out, violation{n.Pos(), "http.NewRequest: use http.NewRequestWithContext so the request joins the caller's trace"})
				}
			}
		case *ast.CompositeLit:
			if isSelector(n.Type, "http", "Client") && !hasKey(n, "Transport") {
				out = append(out, violation{n.Pos(), "http.Client without Transport: set Transport: otelhttp.NewTransport(...)"})
			}
		}
		return true
	})
	return out
}

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg && sel.Sel.Name == name
}

func hasKey(lit *ast.CompositeLit, key string) bool {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
				return true
			}
		}
	}
	return false
}

func TestConventionViolationsDetectsBadCode(t *testing.T) {
	src := `package x

import (
	"log"
	"log/slog"
	"net/http"
)

func f() {
	log.Println("x")
	slog.Info("x")
	http.Get("http://x")
	http.NewRequest("GET", "http://x", nil)
	_ = &http.Client{}
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(conventionViolations(file)); got != 5 {
		t.Errorf("found %d violations, want 5", got)
	}
}
