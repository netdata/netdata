// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	jobmgrImportPath    = "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	lifecycleImportPath = jobmgrImportPath + "/lifecycle"
)

var passivePackages = []string{
	"lifecycle",
	"functions",
	"joboutput",
	"secrets",
	"discovery",
	"composition",
}

var futureOwnerTypes = map[string]struct{}{
	"AdmissionLedger":       {},
	"OperationGeneration":   {},
	"TaskSupervisor":        {},
	"TaskChild":             {},
	"FrameOwner":            {},
	"RunSupervisor":         {},
	"UIDLedger":             {},
	"CommandKernel":         {},
	"ClaimAuthority":        {},
	"KernelLoop":            {},
	"FunctionIngress":       {},
	"FunctionCatalog":       {},
	"HandlerGeneration":     {},
	"JobGeneration":         {},
	"SecretStoreGeneration": {},
	"PipelineGeneration":    {},
	"ProcessComposition":    {},
}

func TestPassiveArchitecturePackages(t *testing.T) {
	root := jobmgrSourceRoot(t)

	for _, name := range passivePackages {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)

			productionFiles := 0
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
					strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				productionFiles++
				checkPassiveFile(t, name, filepath.Join(dir, entry.Name()))
			}
			assert.Positive(t, productionFiles, "passive package must contain production source")
		})
	}
}

func checkPassiveFile(t *testing.T, packageName, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lower := strings.ToLower(string(data))
	for _, forbidden := range []string{"candidate", "jobmgrpoc", "b-o-"} {
		assert.NotContains(t, lower, forbidden, "%s contains an experimental identity", path)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	require.NoError(t, err)

	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			assert.False(t, strings.HasPrefix(decl.Name.Name, "New"),
				"%s constructs runtime authority during passive preparation", path)
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				_, isOwner := futureOwnerTypes[typeSpec.Name.Name]
				assert.False(t, isOwner,
					"%s declares owner %s during passive preparation", path, typeSpec.Name.Name)
			}
		}
	}

	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		require.NoError(t, err)
		checkPassiveImport(t, packageName, importPath, path)
	}
}

func checkPassiveImport(t *testing.T, packageName, importPath, path string) {
	t.Helper()

	if packageName == "lifecycle" {
		assert.NotContains(t, importPath, ".",
			"neutral lifecycle package imports non-standard package %q in %s", importPath, path)
		return
	}
	if packageName == "composition" {
		return
	}
	if importPath == jobmgrImportPath || importPath == lifecycleImportPath {
		return
	}
	assert.False(t, strings.HasPrefix(importPath, jobmgrImportPath+"/"),
		"adapter package %s imports sibling adapter %q in %s", packageName, importPath, path)
}

func jobmgrSourceRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(file)
}
