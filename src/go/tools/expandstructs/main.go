// SPDX-License-Identifier: GPL-3.0-or-later

// Command expandstructs forces keyed struct composite literals to one field per
// line. For every composite literal whose type is a named type (T / pkg.T /
// T[X], optionally behind &) and whose elements are all key: value pairs, it
// rewrites the brace body so each element sits on its own line with a trailing
// comma; gofmt then re-indents and aligns.
//
// It is AST-based, so it never changes tokens. It skips map/slice/array
// literals, positional (unkeyed) literals, empty literals, elided-type nested
// literals, and any literal containing a comment inside its braces (so element
// comments are never dropped). It runs to a fixpoint so nested struct literals
// expand too.
//
// It is the second stage of the recommended Go formatting pipeline:
//
//	golines -m 120 -t 4 -w   (skip if golines is not installed)
//	expandstructs
//	goimports -w
//
// usage: expandstructs <path> [path ...]
//
// A path may be a Go file or a directory; directories are walked for *.go files
// (skipping vendor, testdata, and dot directories).
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: expandstructs <path> [path ...]")
		os.Exit(2)
	}
	for _, path := range collect(os.Args[1:]) {
		if err := expand(path); err != nil {
			fmt.Fprintln(os.Stderr, path+":", err)
			os.Exit(1)
		}
	}
}

// collect resolves the input paths into a list of Go files, walking any
// directories and skipping vendor, testdata, and dot directories.
func collect(paths []string) []string {
	var files []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, path+":", err)
			os.Exit(1)
		}
		if !info.IsDir() {
			files = append(files, path)
			continue
		}
		err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if p != path && (name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".")) {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".go") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, path+":", err)
			os.Exit(1)
		}
	}
	return files
}

// expand rewrites path to a fixpoint so nested struct literals also expand.
func expand(path string) error {
	for {
		changed, err := onePass(path)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
	}
}

func onePass(path string) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return false, err
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	line := func(p token.Pos) int { return fset.Position(p).Line }

	type span struct{ start, end int }
	var comments []span
	for _, cg := range f.Comments {
		comments = append(comments, span{off(cg.Pos()), off(cg.End())})
	}
	commentInside := func(a, b int) bool {
		for _, c := range comments {
			if c.start < b && c.end > a {
				return true
			}
		}
		return false
	}

	type repl struct {
		start, end int
		text       []byte
	}
	var repls []repl
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !namedType(lit.Type) || len(lit.Elts) == 0 {
			return true
		}
		for _, e := range lit.Elts {
			if _, ok := e.(*ast.KeyValueExpr); !ok {
				return true // positional element -> leave alone
			}
		}
		if alreadyOnePerLine(lit, line) {
			return true
		}
		lb, rb := off(lit.Lbrace), off(lit.Rbrace)+1
		if commentInside(lb, rb) {
			return true
		}
		var b bytes.Buffer
		b.WriteString("{\n")
		for _, e := range lit.Elts {
			b.Write(src[off(e.Pos()):off(e.End())])
			b.WriteString(",\n")
		}
		b.WriteString("}")
		repls = append(repls, repl{lb, rb, b.Bytes()})
		return true
	})
	if len(repls) == 0 {
		return false, nil
	}
	sort.Slice(repls, func(i, j int) bool { return repls[i].start < repls[j].start })
	var top []repl
	lastEnd := -1
	for _, r := range repls {
		if r.start >= lastEnd { // outermost only; nested handled on the next pass
			top = append(top, r)
			lastEnd = r.end
		}
	}
	var out bytes.Buffer
	prev := 0
	for _, r := range top {
		out.Write(src[prev:r.start])
		out.Write(r.text)
		prev = r.end
	}
	out.Write(src[prev:])
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return false, err
	}
	return true, exec.Command("gofmt", "-w", path).Run()
}

// namedType reports whether t denotes a named type (so the literal is a struct,
// not a map/slice/array literal).
func namedType(t ast.Expr) bool {
	switch t.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr, *ast.IndexListExpr:
		return true
	}
	return false
}

// alreadyOnePerLine reports whether every element already starts on its own
// line between the braces, so the literal needs no rewrite (and the fixpoint
// terminates).
func alreadyOnePerLine(lit *ast.CompositeLit, line func(token.Pos) int) bool {
	prev := line(lit.Lbrace)
	for _, e := range lit.Elts {
		el := line(e.Pos())
		if el <= prev {
			return false
		}
		prev = el
	}
	return line(lit.Rbrace) > prev
}
