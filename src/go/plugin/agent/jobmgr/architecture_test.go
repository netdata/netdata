// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
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

type ownerDeclaration struct {
	packagePath string
	typeName    string
	methodName  string
	fieldName   string
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

var productionOwnerDeclarations = map[string]ownerDeclaration{
	"admission authority": {
		packagePath: "plugin/agent/jobmgr/lifecycle", typeName: "AdmissionLedger",
	},
	"operation lifecycle": {
		packagePath: "plugin/agent/jobmgr/lifecycle", typeName: "OperationGeneration",
	},
	"kernel lanes": {
		packagePath: "plugin/agent/jobmgr", typeName: "CommandKernel",
	},
	"claim authority": {
		packagePath: "plugin/agent/jobmgr", typeName: "claimAuthority",
	},
	"kernel loop": {
		packagePath: "plugin/agent/jobmgr", typeName: "KernelLoop",
	},
	"task supervisor": {
		packagePath: "plugin/agent/jobmgr/lifecycle", typeName: "TaskSupervisor",
	},
	"task child": {
		packagePath: "plugin/agent/jobmgr/lifecycle",
		typeName:    "TaskSupervisor",
		methodName:  "runChild",
	},
	"frame authority": {
		packagePath: "plugin/agent/jobmgr/lifecycle", typeName: "FrameOwner",
	},
	"run authority": {
		packagePath: "plugin/agent/jobmgr/lifecycle", typeName: "RunSupervisor",
	},
	"uid authority": {
		packagePath: "plugin/agent/jobmgr/lifecycle", typeName: "UIDLedger",
	},
	"function input": {
		packagePath: "plugin/framework/functions", typeName: "InputCapsule",
	},
	"function catalog": {
		packagePath: "plugin/agent/jobmgr/functions", typeName: "Catalog",
	},
	"handler generation": {
		packagePath: "plugin/agent/jobmgr/functions", typeName: "handlerGeneration",
	},
	"function publication": {
		packagePath: "plugin/agent/jobmgr/functions", typeName: "Controller",
	},
	"dynamic configuration graph": {
		packagePath: "plugin/framework/dyncfg", typeName: "Graph",
	},
	"job factory": {
		packagePath: "plugin/agent/jobmgr/joboutput", typeName: "Factory",
	},
	"configuration factory": {
		packagePath: "plugin/agent/jobmgr/joboutput", typeName: "ConfigModuleFactory",
	},
	"job generation": {
		packagePath: "plugin/agent/jobmgr/joboutput", typeName: "JobGeneration",
	},
	"v1 runtime": {
		packagePath: "plugin/framework/jobruntime", typeName: "V1Runtime",
	},
	"v2 runtime": {
		packagePath: "plugin/framework/jobruntime", typeName: "V2Runtime",
	},
	"v2 scope": {
		packagePath: "plugin/framework/jobruntime", typeName: "jobV2ScopeState",
	},
	"v2 host": {
		packagePath: "plugin/framework/jobruntime", typeName: "jobV2HostState",
	},
	"prepared vnode frame": {
		packagePath: "plugin/agent/jobmgr/joboutput", typeName: "PreparedVNodeFrame",
	},
	"job vnode state": {
		packagePath: "plugin/agent/jobmgr/joboutput", typeName: "ConstructedJob",
		fieldName: "ReleaseVNode",
	},
	"secret resolver": {
		packagePath: "plugin/agent/secrets/resolver", typeName: "AtomicResolver",
	},
	"secret creator catalog": {
		packagePath: "plugin/agent/secrets/secretstore", typeName: "CreatorCatalog",
	},
	"secret store authority": {
		packagePath: "plugin/agent/secrets/secretstore", typeName: "SecretStore",
	},
	"prepared secret mutation": {
		packagePath: "plugin/agent/secrets/secretstore", typeName: "PreparedSecretMutation",
	},
	"secret dependency index": {
		packagePath: "plugin/agent/jobmgr/secrets", typeName: "SecretDependencyIndex",
	},
	"secret restart command": {
		packagePath: "plugin/agent/jobmgr/secrets", typeName: "SecretRestartCommand",
	},
	"discovery provider catalog": {
		packagePath: "plugin/agent/discovery", typeName: "ProviderCatalog",
	},
	"discovery pipeline generation": {
		packagePath: "plugin/agent/discovery", typeName: "PipelineGeneration",
	},
	"discovery decision index": {
		packagePath: "plugin/agent/jobmgr/discovery", typeName: "DecisionIndex",
	},
	"configured vnode authority": {
		packagePath: "plugin/agent/jobmgr/discovery", typeName: "VNodeConfiguration",
	},
	"vnode metadata authority": {
		packagePath: "plugin/framework/vnoderegistry", typeName: "Registry",
	},
	"process authority": {
		packagePath: "plugin/agent/jobmgr/composition", typeName: "Process",
	},
	"module catalog": {
		packagePath: "plugin/agent/jobmgr/composition",
		typeName:    "Config",
		fieldName:   "Modules",
	},
}

func TestActiveArchitecturePackages(t *testing.T) {
	root := jobmgrSourceRoot(t)
	foundOwners := make(map[string]string, len(requiredLifecycleOwners))

	for name, rule := range activePackageRules {
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
				checkActiveFile(t, name, rule, filepath.Join(dir, entry.Name()), foundOwners)
			}
			require.NotEqualValues(t, 0, productionFiles)
		})
	}

	for owner := range requiredLifecycleOwners {
		assert.NotEqualValues(t, "", foundOwners[owner])
	}
}

func TestProductionOwnerManifestHasConcreteDeclarations(t *testing.T) {
	const productionOwnerCount = 37
	require.EqualValues(t, productionOwnerCount, len(productionOwnerDeclarations))
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	for role, owner := range productionOwnerDeclarations {
		t.Run(role, func(t *testing.T) {
			found, err := findOwnerDeclaration(filepath.Join(root, owner.packagePath), owner)
			require.NoError(t, err)
			require.True(t, found)
		})
	}
}

func TestProductionSourceClosure(t *testing.T) {
	tests := map[string]func(t *testing.T){
		"exact Job Manager tree": assertExactJobManagerTree,
		"no ignored Go source":   assertNoIgnoredJobManagerSource,
		"retired owners absent":  assertRetiredOwnersAbsent,
	}
	for name, run := range tests {
		t.Run(name, run)
	}
}

func assertExactJobManagerTree(t *testing.T) {
	t.Helper()
	allowed := map[string]struct{}{
		"ARCHITECTURE.md":                          {},
		"architecture_test.go":                     {},
		"claim_authority.go":                       {},
		"claim_authority_test.go":                  {},
		"command_ports.go":                         {},
		"command_ports_test.go":                    {},
		"composite.go":                             {},
		"composite_kernel.go":                      {},
		"composite_kernel_test.go":                 {},
		"composition":                              {},
		"discovery":                                {},
		"doc.go":                                   {},
		"function_catalog_port.go":                 {},
		"function_catalog_port_test.go":            {},
		"functions":                                {},
		"joboutput":                                {},
		"kernel.go":                                {},
		"kernel_fairness_test.go":                  {},
		"kernel_function_catalog_external_test.go": {},
		"kernel_function_cleanup_queue_test.go":    {},
		"kernel_hotpath_bench_test.go":             {},
		"kernel_lifecycle_test.go":                 {},
		"kernel_prepared_wait_test.go":             {},
		"lifecycle":                                {},
		"plan.go":                                  {},
		"production_cases_test.go":                 {},
		"secrets":                                  {},
	}
	entries, err := os.ReadDir(jobmgrSourceRoot(t))
	require.NoError(t, err)
	for _, entry := range entries {
		_, ok := allowed[entry.Name()]
		assert.True(t, ok)
	}
	for name := range allowed {

		_, statErr := os.Stat(filepath.Join(jobmgrSourceRoot(t), name))
		assert.NoError(t, statErr)

	}
}

func assertNoIgnoredJobManagerSource(t *testing.T) {
	t.Helper()
	err := filepath.WalkDir(
		jobmgrSourceRoot(t),
		func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			assert.NotContains(t, string(data), "//go:build "+"ignore")
			return nil
		},
	)
	require.NoError(t, err)
}

func assertRetiredOwnersAbsent(t *testing.T) {
	t.Helper()
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	tests := map[string]struct {
		path      string
		types     map[string]struct{}
		functions map[string]struct{}
	}{
		"Functions Manager": {
			path: "plugin/framework/functions",
			types: map[string]struct{}{
				"Manager": {}, "LaneKeyDeriver": {}, "inputParser": {},
				"keyScheduler": {},
			},
			functions: map[string]struct{}{"NewManager": {}},
		},
		"SecretStore Service": {
			path: "plugin/agent/secrets/secretstore",
			types: map[string]struct{}{
				"Service": {}, "Snapshot": {}, "inMemoryService": {},
				"runtimeResolver": {},
			},
			functions: map[string]struct{}{"NewService": {}},
		},
		"recursive resolver": {
			path:      "plugin/agent/secrets/resolver",
			types:     map[string]struct{}{"Resolver": {}},
			functions: map[string]struct{}{"New": {}},
		},
		"file persister lifecycle": {
			path: "plugin/framework/filepersister",
			types: map[string]struct{}{
				"Persister": {}, "Data": {},
			},
			functions: map[string]struct{}{"New": {}},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assertNoDeclarations(
				t,
				filepath.Join(root, test.path),
				test.types,
				test.functions,
			)
		})
	}
}

func assertNoDeclarations(
	t *testing.T,
	dir string,
	types map[string]struct{},
	functions map[string]struct{},
) {
	t.Helper()
	files, err := productionGoFiles(dir)
	require.NoError(t, err)
	for _, path := range files {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, err)
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if declaration.Recv == nil {
					_, banned := functions[declaration.Name.Name]
					assert.False(t, banned)
				}
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok {
						continue
					}

					_, banned := types[typeSpec.Name.Name]
					assert.False(t, banned)
				}
			}
		}
	}
}

func findOwnerDeclaration(
	dir string,
	owner ownerDeclaration,
) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() ||
			!strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(
			token.NewFileSet(),
			path,
			nil,
			0,
		)
		if err != nil {
			return false, err
		}
		for _, declaration := range file.Decls {
			if owner.methodName != "" {
				method, ok := declaration.(*ast.FuncDecl)
				if ok &&
					method.Name.Name == owner.methodName &&
					receiverTypeName(method.Recv) == owner.typeName {
					return true, nil
				}
				continue
			}
			generic, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range generic.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != owner.typeName {
					continue
				}
				if owner.fieldName == "" {
					return true, nil
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						if name.Name == owner.fieldName {
							return true, nil
						}
					}
				}
			}
		}
	}
	return false, nil
}

func receiverTypeName(receiver *ast.FieldList) string {
	if receiver == nil || len(receiver.List) != 1 {
		return ""
	}
	expression := receiver.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
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
	require.NoError(t, err)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	require.NoError(t, err)

	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			assert.False(t, decl.Recv == nil && decl.Name.Name == "init")
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

				previous := foundOwners[typeSpec.Name.Name]
				assert.EqualValues(t, "", previous)

				foundOwners[typeSpec.Name.Name] = path
			}
		}
	}

	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		require.NoError(t, err)
		assert.NotEqualValues(t, "C", importPath)
		checkActiveImport(t, packageName, rule, importPath, path)
	}
}

func TestNoExperimentalIdentity(t *testing.T) {
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	forbidden := []string{
		"jobmgr" + "poc",
		"jobmgr" + "eval",
		"candidate" + "-b",
		"b-" + "o-",
	}
	for _, dir := range []string{"cmd", "plugin/agent", "plugin/framework"} {
		err := filepath.WalkDir(
			filepath.Join(root, dir),
			func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() ||
					!strings.HasSuffix(entry.Name(), ".go") {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				lower := strings.ToLower(string(data))
				for _, token := range forbidden {
					assert.NotContains(t, lower, token)
				}
				return nil
			},
		)
		require.NoError(t, err)
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
		assert.False(t, strings.HasPrefix(importPath, "github.com/netdata/netdata/go/plugins/"))
		return
	}
	if rule.mayImportSiblings {
		return
	}
	if importPath == jobmgrImportPath || importPath == lifecycleImportPath {
		return
	}
	assert.False(t, strings.HasPrefix(importPath, jobmgrImportPath+"/"))
}

type constructionCounts struct {
	agentNew           int
	hostRun            int
	processNew         int
	retiredNew         int
	commandKernel      int
	rootHandoff        bool
	agentProcessRun    bool
	compositionRunPath uint8
}

type constructionContract struct {
	agentNew      int
	hostRun       int
	processNew    int
	retiredNew    int
	commandKernel int
}

const (
	compositionPathNewProcessCore uint8 = 1 << iota
	compositionPathPublicRun
	compositionPathCoreNewRun
	compositionPathGeneration
	compositionPathKernel

	completeCompositionRunPath = compositionPathNewProcessCore |
		compositionPathPublicRun |
		compositionPathCoreNewRun |
		compositionPathGeneration |
		compositionPathKernel
)

func TestProductionConstructionChain(t *testing.T) {
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	roots := map[string]string{
		"godplugin":      "cmd/godplugin/main.go",
		"ibmdplugin":     "cmd/ibmdplugin/main.go",
		"scriptsdplugin": "cmd/scriptsdplugin/main.go",
	}
	for name, path := range roots {
		t.Run(name, func(t *testing.T) {
			counts, err := inspectConstructionFiles(
				[]string{filepath.Join(root, path)},
			)
			require.NoError(t, err)

			require.NoError(t, validateConstructionContract(
				counts,
				constructionContract{agentNew: 1, hostRun: 1},
			),
			)
		})
	}

	packages := map[string]struct {
		path     string
		contract constructionContract
	}{
		"agent": {
			path: "plugin/agent",
			contract: constructionContract{
				processNew: 1,
			},
		},
		"composition": {
			path: "plugin/agent/jobmgr/composition",
			contract: constructionContract{
				commandKernel: 1,
			},
		},
	}
	for name, test := range packages {
		t.Run(name, func(t *testing.T) {
			files, err := productionGoFiles(filepath.Join(root, test.path))
			require.NoError(t, err)
			counts, err := inspectConstructionFiles(files)
			require.NoError(t, err)

			require.NoError(t, validateConstructionContract(counts, test.contract))
		})
	}
}

func TestProductionCompositionConstructsOneSecretAuthoritySet(
	t *testing.T,
) {
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	files, err := productionGoFiles(filepath.Join(root, "plugin/agent/jobmgr/composition"))
	require.NoError(t, err)
	calls := map[string]struct {
		importPath string
		function   string
		want       int
	}{
		"Store authority": {
			importPath: pluginsImportRoot +
				"plugin/agent/secrets/secretstore",
			function: "NewSecretStore", want: 1,
		},
		"dependency authority": {
			importPath: jobmgrImportPath + "/secrets",
			function:   "NewSecretDependencyIndex", want: 1,
		},
		"secret controller": {
			importPath: jobmgrImportPath + "/secrets",
			function:   "NewController", want: 1,
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			got, err := countImportedCalls(files, call.importPath, call.function)
			require.NoError(t, err)
			require.EqualValues(t, call.want, got)
		})
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		source := string(data)
		for _, forbidden := range []string{
			jobmgrImportPath + "/secretsctl",
			"secretstore.NewService(",
		} {
			require.NotContains(t, source, forbidden)
		}
	}
}

func TestProductionCompositionConstructsOneDiscoveryAuthoritySet(
	t *testing.T,
) {
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	files, err := productionGoFiles(filepath.Join(root, "plugin/agent/jobmgr/composition"))
	require.NoError(t, err)
	calls := map[string]struct {
		importPath string
		function   string
		want       int
	}{
		"pipeline generation": {
			importPath: pluginsImportRoot +
				"plugin/agent/discovery",
			function: "NewPipelineGeneration", want: 1,
		},
		"decision index": {
			importPath: jobmgrImportPath + "/discovery",
			function:   "NewDecisionIndex", want: 1,
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			got, err := countImportedCalls(files, call.importPath, call.function)
			require.NoError(t, err)
			require.EqualValues(t, call.want, got)
		})
	}
}

func TestProductionConstructionGuardRejectsAdversarialSources(t *testing.T) {
	tests := map[string]struct {
		source   string
		contract constructionContract
	}{
		"missing host handoff": {
			source: `package main
				import "github.com/netdata/netdata/go/plugins/plugin/agent"
				func main() { _ = agent.New(agent.Config{}) }`,
			contract: constructionContract{agentNew: 1, hostRun: 1},
		},
		"duplicate agent construction": {
			source: `package main
				import (
					"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
					"github.com/netdata/netdata/go/plugins/plugin/agent"
				)
				func main() {
					a := agent.New(agent.Config{})
					_ = agent.New(agent.Config{})
					agenthost.Run(a)
				}`,
			contract: constructionContract{agentNew: 1, hostRun: 1},
		},
		"retired manager beside process": {
			source: `package agent
				import (
					"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
					"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition"
				)
				func run() {
					_, _ = composition.NewProcess(composition.Config{})
					_ = jobmgr.New(jobmgr.Config{})
				}`,
			contract: constructionContract{processNew: 1},
		},
		"root constructs composition directly": {
			source: `package main
				import (
					"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
					"github.com/netdata/netdata/go/plugins/plugin/agent"
					"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition"
				)
				func main() {
					a := agent.New(agent.Config{})
					_, _ = composition.NewProcess(composition.Config{})
					agenthost.Run(a)
				}`,
			contract: constructionContract{agentNew: 1, hostRun: 1},
		},
		"disconnected root calls": {
			source: `package main
				import (
					"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
					"github.com/netdata/netdata/go/plugins/plugin/agent"
				)
				func construct() *agent.Agent {
					return agent.New(agent.Config{})
				}
				func run(a *agent.Agent) {
					agenthost.Run(a)
				}
				func main() {}`,
			contract: constructionContract{agentNew: 1, hostRun: 1},
		},
		"disconnected Agent process construction": {
			source: `package agent
				import "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition"
				func construct() {
					_, _ = composition.NewProcess(composition.Config{})
				}
				func (a *Agent) run() error { return nil }`,
			contract: constructionContract{processNew: 1},
		},
		"disconnected composition Kernel construction": {
			source: `package composition
				import "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
				func construct() {
					_, _ = jobmgr.NewCommandKernel(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				}`,
			contract: constructionContract{commandKernel: 1},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			counts, err := inspectConstructionSource(name+".go", test.source)
			require.NoError(t, err)

			require.Error(t, validateConstructionContract(counts, test.contract))
		})
	}
}

func productionGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() ||
			!strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		matched, err := build.Default.MatchFile(dir, entry.Name())
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	if len(paths) == 0 {
		return nil, errors.New("architecture guard: no production Go files")
	}
	return paths, nil
}

func countImportedCalls(
	paths []string,
	importPath string,
	function string,
) (int, error) {
	total := 0
	for _, path := range paths {
		file, err := parser.ParseFile(
			token.NewFileSet(),
			path,
			nil,
			0,
		)
		if err != nil {
			return 0, err
		}
		qualifiers := make(map[string]struct{})
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return 0, err
			}
			if path != importPath {
				continue
			}
			name := filepath.Base(path)
			if imported.Name != nil {
				name = imported.Name.Name
			}
			if name == "." {
				return 0, fmt.Errorf("architecture guard: dot import %q", importPath)
			}
			if name != "_" {
				qualifiers[name] = struct{}{}
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
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, ok := qualifiers[qualifier.Name]; ok {
				total++
			}
			return true
		})
	}
	return total, nil
}

func inspectConstructionFiles(paths []string) (constructionCounts, error) {
	var total constructionCounts
	for _, path := range paths {
		counts, err := inspectConstructionSource(path, nil)
		if err != nil {
			return constructionCounts{}, err
		}
		total.agentNew += counts.agentNew
		total.hostRun += counts.hostRun
		total.processNew += counts.processNew
		total.retiredNew += counts.retiredNew
		total.commandKernel += counts.commandKernel
		total.rootHandoff = total.rootHandoff || counts.rootHandoff
		total.agentProcessRun =
			total.agentProcessRun || counts.agentProcessRun
		total.compositionRunPath |= counts.compositionRunPath
	}
	return total, nil
}

func inspectConstructionSource(
	filename string,
	source any,
) (constructionCounts, error) {
	file, err := parser.ParseFile(
		token.NewFileSet(),
		filename,
		source,
		0,
	)
	if err != nil {
		return constructionCounts{}, err
	}
	imports := make(map[string]string, len(file.Imports))
	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return constructionCounts{}, err
		}
		name := filepath.Base(importPath)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		switch name {
		case "_":
			continue
		case ".":
			if isConstructionImport(importPath) {
				return constructionCounts{}, fmt.Errorf("architecture guard: dot import %q", importPath)
			}
		default:
			imports[name] = importPath
		}
	}
	var counts constructionCounts
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch imports[qualifier.Name] {
		case agentImportPath:
			if selector.Sel.Name == "New" {
				counts.agentNew++
			}
		case agentHostImportPath:
			if selector.Sel.Name == "Run" {
				counts.hostRun++
			}
		case compositionImportPath:
			if selector.Sel.Name == "NewProcess" {
				counts.processNew++
			}
		case jobmgrImportPath:
			switch selector.Sel.Name {
			case "New":
				counts.retiredNew++
			case "NewCommandKernel":
				counts.commandKernel++
			}
		}
		return true
	})
	counts.rootHandoff = constructionRootHandoff(file, imports)
	counts.agentProcessRun = constructionAgentProcessRun(file, imports)
	counts.compositionRunPath = constructionCompositionRunPath(file, imports)
	return counts, nil
}

func constructionRootHandoff(
	file *ast.File,
	imports map[string]string,
) bool {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name.Name != "main" ||
			function.Body == nil {
			continue
		}
		constructed := make(map[string]struct{})
		inspectConstructionBody(function.Body, func(call *ast.CallExpr) {
			if !isImportedConstructionCall(
				call,
				imports,
				agentImportPath,
				"New",
			) {
				return
			}
			assignment, ok := parentAssignment(function.Body, call)
			if !ok {
				return
			}
			constructed[assignment] = struct{}{}
		})
		connected := false
		inspectConstructionBody(function.Body, func(call *ast.CallExpr) {
			if connected ||
				!isImportedConstructionCall(
					call,
					imports,
					agentHostImportPath,
					"Run",
				) ||
				len(call.Args) != 1 {
				return
			}
			argument, ok := call.Args[0].(*ast.Ident)
			if !ok {
				return
			}
			_, connected = constructed[argument.Name]
		})
		return connected
	}
	return false
}

func constructionAgentProcessRun(
	file *ast.File,
	imports map[string]string,
) bool {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "run" ||
			receiverType(function) != "Agent" ||
			function.Body == nil {
			continue
		}
		constructed := make(map[string]struct{})
		inspectConstructionBody(function.Body, func(call *ast.CallExpr) {
			if !isImportedConstructionCall(
				call,
				imports,
				compositionImportPath,
				"NewProcess",
			) {
				return
			}
			assignment, ok := parentAssignment(function.Body, call)
			if ok {
				constructed[assignment] = struct{}{}
			}
		})
		connected := false
		inspectConstructionBody(function.Body, func(call *ast.CallExpr) {
			if connected {
				return
			}
			parts := selectorParts(call.Fun)
			if len(parts) != 2 || parts[1] != "Run" {
				return
			}
			_, connected = constructed[parts[0]]
		})
		return connected
	}
	return false
}

func constructionCompositionRunPath(
	file *ast.File,
	imports map[string]string,
) uint8 {
	var path uint8
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		receiver := receiverType(function)
		receiverName := receiverName(function)
		switch {
		case function.Recv == nil && function.Name.Name == "NewProcess":
			if bodyCallsIdentifier(function.Body, "newProcessCore") {
				path |= compositionPathNewProcessCore
			}
		case receiver == "Process" && function.Name.Name == "Run":
			if bodyCallsSelector(
				function.Body,
				[]string{receiverName, "core", "run"},
			) {
				path |= compositionPathPublicRun
			}
		case receiver == "processCore" && function.Name.Name == "run":
			if bodyCallsSelector(
				function.Body,
				[]string{receiverName, "newRun"},
			) {
				path |= compositionPathCoreNewRun
			}
		case receiver == "processCore" && function.Name.Name == "newRun":
			if bodyCallsIdentifier(function.Body, "newRunGeneration") {
				path |= compositionPathGeneration
			}
		case function.Recv == nil &&
			function.Name.Name == "newRunGeneration":
			if bodyCallsImported(
				function.Body,
				imports,
				jobmgrImportPath,
				"NewCommandKernel",
			) {
				path |= compositionPathKernel
			}
		}
	}
	return path
}

func inspectConstructionBody(
	body *ast.BlockStmt,
	visit func(*ast.CallExpr),
) {
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			visit(call)
		}
		return true
	})
}

func parentAssignment(
	body *ast.BlockStmt,
	target *ast.CallExpr,
) (string, bool) {
	var name string
	ast.Inspect(body, func(node ast.Node) bool {
		if name != "" {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for index, expression := range assignment.Rhs {
			if expression != target || index >= len(assignment.Lhs) {
				continue
			}
			identifier, ok := assignment.Lhs[index].(*ast.Ident)
			if ok && identifier.Name != "_" {
				name = identifier.Name
			}
			return false
		}
		return true
	})
	return name, name != ""
}

func isImportedConstructionCall(
	call *ast.CallExpr,
	imports map[string]string,
	importPath string,
	function string,
) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != function {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && imports[qualifier.Name] == importPath
}

func bodyCallsImported(
	body *ast.BlockStmt,
	imports map[string]string,
	importPath string,
	function string,
) bool {
	found := false
	inspectConstructionBody(body, func(call *ast.CallExpr) {
		found = found || isImportedConstructionCall(
			call,
			imports,
			importPath,
			function,
		)
	})
	return found
}

func bodyCallsIdentifier(body *ast.BlockStmt, name string) bool {
	found := false
	inspectConstructionBody(body, func(call *ast.CallExpr) {
		identifier, ok := call.Fun.(*ast.Ident)
		found = found || ok && identifier.Name == name
	})
	return found
}

func bodyCallsSelector(body *ast.BlockStmt, want []string) bool {
	found := false
	inspectConstructionBody(body, func(call *ast.CallExpr) {
		found = found || reflect.DeepEqual(selectorParts(call.Fun), want)
	})
	return found
}

func selectorParts(expression ast.Expr) []string {
	switch value := expression.(type) {
	case *ast.Ident:
		return []string{value.Name}
	case *ast.SelectorExpr:
		return append(selectorParts(value.X), value.Sel.Name)
	default:
		return nil
	}
}

func receiverType(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return ""
	}
	expression := function.Recv.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func receiverName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 ||
		len(function.Recv.List[0].Names) != 1 {
		return ""
	}
	return function.Recv.List[0].Names[0].Name
}

func isConstructionImport(importPath string) bool {
	switch importPath {
	case agentImportPath,
		agentHostImportPath,
		compositionImportPath,
		jobmgrImportPath:
		return true
	default:
		return false
	}
}

func validateConstructionContract(
	counts constructionCounts,
	want constructionContract,
) error {
	var result error
	if counts.agentNew != want.agentNew {
		result = errors.Join(result, fmt.Errorf("agent.New calls=%d want=%d", counts.agentNew, want.agentNew))
	}
	if counts.hostRun != want.hostRun {
		result = errors.Join(result, fmt.Errorf("agenthost.Run calls=%d want=%d", counts.hostRun, want.hostRun))
	}
	if counts.processNew != want.processNew {
		result = errors.Join(result, fmt.Errorf("composition.NewProcess calls=%d want=%d", counts.processNew, want.processNew))
	}
	if counts.retiredNew != want.retiredNew {
		result = errors.Join(result, fmt.Errorf("retired jobmgr.New calls=%d want=%d", counts.retiredNew, want.retiredNew))
	}
	if counts.commandKernel != want.commandKernel {
		result = errors.Join(result, fmt.Errorf(
			"jobmgr.NewCommandKernel calls=%d want=%d",
			counts.commandKernel,
			want.commandKernel,
		))
	}
	if want.agentNew != 0 && want.hostRun != 0 &&
		!counts.rootHandoff {
		result = errors.Join(result, errors.New("agent.New result does not reach agenthost.Run from main"))
	}
	if want.processNew != 0 && !counts.agentProcessRun {
		result = errors.Join(result, errors.New("Agent.run does not run its composition.NewProcess result"))
	}
	if want.commandKernel != 0 &&
		counts.compositionRunPath != completeCompositionRunPath {
		result = errors.Join(result, fmt.Errorf(
			"production composition path=0x%x want=0x%x",
			counts.compositionRunPath,
			completeCompositionRunPath,
		))
	}
	return result
}

func TestWorkPlanCannotCarryKernelLoopAbandonCallback(t *testing.T) {
	_, found := reflect.TypeFor[jobmgr.WorkPlan]().FieldByName("Abandon")
	require.False(t, found)
}

func TestCommandKernelUsesSourceSpecificPlanningPorts(t *testing.T) {
	constructor := reflect.ValueOf(jobmgr.NewCommandKernel).Type()
	planner := reflect.TypeFor[jobmgr.Planner]()
	functionCatalog := reflect.TypeFor[jobmgr.FunctionCatalogPort]()
	require.False(t, constructor.NumIn() < 2 ||
		constructor.In(constructor.NumIn()-2) != planner ||
		constructor.In(constructor.NumIn()-1) != functionCatalog)
	for in := range constructor.Ins() {
		require.NotEqualValues(t, reflect.Map, in.Kind())
	}

	data, err := os.ReadFile(filepath.Join(jobmgrSourceRoot(t), "kernel.go"))
	require.NoError(t, err)
	source := string(data)
	for _, forbidden := range []string{
		"map[lifecycle.Source]Planner",
		"planners[request.Source]",
		"preparePlan(request Request)",
	} {
		require.NotContains(t, source, forbidden)
	}
}

func jobmgrSourceRoot(t *testing.T) string {
	t.Helper()

	root, err := os.Getwd()
	require.NoError(t, err)
	return root
}
