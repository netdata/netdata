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
)

const (
	jobmgrImportPath    = "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	lifecycleImportPath = jobmgrImportPath + "/lifecycle"
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

var requiredLifecycleOwners = map[string]struct{}{
	"AdmissionLedger":     {},
	"OperationGeneration": {},
	"TaskSupervisor":      {},
	"FrameOwner":          {},
	"RunSupervisor":       {},
	"UIDLedger":           {},
}

func TestActiveArchitecturePackages(t *testing.T) {
	root := jobmgrSourceRoot(t)
	foundOwners := make(map[string]string, len(requiredLifecycleOwners))

	for name, rule := range activePackageRules {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}

			productionFiles := 0
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
					strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				productionFiles++
				checkActiveFile(t, name, rule, filepath.Join(dir, entry.Name()), foundOwners)
			}
			if productionFiles == 0 {
				t.Fatal("active package must contain production source")
			}
		})
	}

	for owner := range requiredLifecycleOwners {
		if foundOwners[owner] == "" {
			t.Errorf("lifecycle owner %s is not declared", owner)
		}
	}
}

func checkActiveFile(
	t *testing.T,
	packageName string,
	rule packageRule,
	path string,
	foundOwners map[string]string,
) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(data))
	for _, forbidden := range []string{"candidate", "jobmgrpoc", "b-o-"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("%s contains experimental identity %q", path, forbidden)
		}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Recv == nil && decl.Name.Name == "init" {
				t.Errorf("%s declares init", path)
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if packageName != "lifecycle" {
					continue
				}
				if _, required := requiredLifecycleOwners[typeSpec.Name.Name]; !required {
					continue
				}
				if previous := foundOwners[typeSpec.Name.Name]; previous != "" {
					t.Errorf("lifecycle owner %s is declared in both %s and %s",
						typeSpec.Name.Name, previous, path)
				}
				foundOwners[typeSpec.Name.Name] = path
			}
		}
	}

	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if importPath == "C" {
			t.Errorf("%s imports cgo", path)
		}
		checkActiveImport(t, packageName, rule, importPath, path)
	}
}

func checkActiveImport(
	t *testing.T,
	packageName string,
	rule packageRule,
	importPath string,
	path string,
) {
	t.Helper()

	if rule.neutral {
		if strings.HasPrefix(importPath, "github.com/netdata/netdata/go/plugins/") {
			t.Errorf("neutral lifecycle package imports Agent/domain package %q in %s",
				importPath, path)
		}
		return
	}
	if rule.mayImportSiblings {
		return
	}
	if importPath == jobmgrImportPath || importPath == lifecycleImportPath {
		return
	}
	if strings.HasPrefix(importPath, jobmgrImportPath+"/") {
		t.Errorf("adapter package %s imports sibling adapter %q in %s",
			packageName, importPath, path)
	}
}

func TestCompositionIsPrivateBeforeAtomicCut(t *testing.T) {
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	paths := map[string]string{
		"godplugin":      "cmd/godplugin/main.go",
		"ibmdplugin":     "cmd/ibmdplugin/main.go",
		"scriptsdplugin": "cmd/scriptsdplugin/main.go",
		"agent":          "plugin/agent/agent.go",
		"agent host":     "cmd/internal/agenthost/host.go",
	}

	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, filepath.Join(root, path), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imported := range file.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if importPath == jobmgrImportPath+"/composition" {
					t.Fatalf("%s publishes composition before the atomic cut", path)
				}
			}
		})
	}
}

func jobmgrSourceRoot(t *testing.T) string {
	t.Helper()

	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return root
}
