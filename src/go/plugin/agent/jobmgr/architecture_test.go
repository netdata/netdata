// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pluginsImportRoot     = "github.com/netdata/netdata/go/plugins/"
	jobmgrImportPath      = pluginsImportRoot + "plugin/agent/jobmgr"
	lifecycleImportPath   = jobmgrImportPath + "/lifecycle"
	agentImportPath       = pluginsImportRoot + "plugin/agent"
	agentHostImportPath   = pluginsImportRoot + "cmd/internal/agenthost"
	compositionImportPath = jobmgrImportPath + "/composition"
)

type packageRule struct {
	neutral           bool
	mayImportSiblings bool
}

var activePackageRules = map[string]packageRule{
	"lifecycle":   {neutral: true},
	"functions":   {},
	"joboutput":   {},
	"secrets":     {},
	"discovery":   {},
	"composition": {mayImportSiblings: true},
}

func TestActivePackageLayering(t *testing.T) {
	root := jobmgrSourceRoot(t)
	for name, rule := range activePackageRules {
		t.Run(name, func(t *testing.T) {
			files, err := productionGoFiles(filepath.Join(root, name))
			require.NoError(t, err)
			require.NotEmpty(t, files)

			for _, path := range files {
				file := parseGoFile(t, path)
				for _, imported := range file.Imports {
					importPath, err := strconv.Unquote(imported.Path.Value)
					require.NoError(t, err)
					require.NotEqual(t, "C", importPath)
					checkActiveImport(t, rule, importPath)
				}
			}
		})
	}
}

func TestCommandKernelRoutesOperationActionsThroughOwnershipGate(t *testing.T) {
	files, err := productionGoFiles(jobmgrSourceRoot(t))
	require.NoError(t, err)

	directSenders := make(map[string]int)
	for _, path := range files {
		file := parseGoFile(t, path)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "SendAction" {
					directSenders[function.Name.Name]++
				}
				return true
			})
		}
	}

	// These are the ownership boundary itself and the four lifecycle-event
	// adapters that are allowed to enter it.
	require.Equal(t, map[string]int{
		"completeRunFinalizer":    1,
		"completeShutdownBarrier": 1,
		"completeTask":            1,
		"sendOperationAction":     1,
		"sendShutdownAction":      1,
	}, directSenders)
}

func TestProductionConstructionBoundaries(t *testing.T) {
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	roots := map[string]string{
		"godplugin":      "cmd/godplugin/main.go",
		"ibmdplugin":     "cmd/ibmdplugin/main.go",
		"scriptsdplugin": "cmd/scriptsdplugin/main.go",
	}
	for name, path := range roots {
		t.Run(name, func(t *testing.T) {
			files := []string{filepath.Join(root, path)}
			assertImportedCallCount(t, files, agentImportPath, "New", 1)
			assertImportedCallCount(t, files, agentHostImportPath, "Run", 1)
		})
	}

	files, err := productionGoFiles(filepath.Join(root, "plugin/agent"))
	require.NoError(t, err)
	assertImportedCallCount(t, files, compositionImportPath, "NewProcess", 1)

	files, err = productionGoFiles(filepath.Join(root, "plugin/agent/jobmgr/composition"))
	require.NoError(t, err)
	assertImportedCallCount(t, files, jobmgrImportPath, "NewCommandKernel", 1)
}

func assertImportedCallCount(t *testing.T, files []string, importPath, function string, want int) {
	t.Helper()
	count := 0
	for _, path := range files {
		file := parseGoFile(t, path)
		aliases := make(map[string]struct{})
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			require.NoError(t, err)
			if path != importPath {
				continue
			}
			alias := filepath.Base(path)
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias != "_" && alias != "." {
				aliases[alias] = struct{}{}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != function {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, ok := aliases[identifier.Name]; ok {
				count++
			}
			return true
		})
	}
	assert.Equal(t, want, count, "%s.%s", importPath, function)
}

func checkActiveImport(t *testing.T, rule packageRule, importPath string) {
	t.Helper()
	if rule.neutral {
		assert.False(t, strings.HasPrefix(importPath, pluginsImportRoot))
		return
	}
	if rule.mayImportSiblings || importPath == jobmgrImportPath || importPath == lifecycleImportPath {
		return
	}
	assert.False(t, strings.HasPrefix(importPath, jobmgrImportPath+"/"))
}

func productionGoFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		files = append(files, filepath.Join(root, entry.Name()))
	}
	return files, nil
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	return file
}

func jobmgrSourceRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	require.NoError(t, err)
	return root
}
