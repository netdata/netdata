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

var checkpointOwnerDeclarations = map[string]ownerDeclaration{
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

func TestCheckpointOwnerManifestHasConcreteDeclarations(t *testing.T) {
	const checkpointOwnerCount = 37
	if len(checkpointOwnerDeclarations) != checkpointOwnerCount {
		t.Fatalf(
			"checkpoint owner declarations=%d want=%d",
			len(checkpointOwnerDeclarations),
			checkpointOwnerCount,
		)
	}
	root := filepath.Clean(filepath.Join(jobmgrSourceRoot(t), "../../.."))
	for role, owner := range checkpointOwnerDeclarations {
		t.Run(role, func(t *testing.T) {
			found, err := findOwnerDeclaration(
				filepath.Join(root, owner.packagePath),
				owner,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatalf(
					"%s has no matching declaration in %s",
					role,
					owner.packagePath,
				)
			}
		})
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
	if err != nil {
		t.Fatal(err)
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
					if strings.Contains(lower, token) {
						t.Errorf(
							"%s contains experimental identity %q",
							path,
							token,
						)
					}
				}
				return nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
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

type constructionCounts struct {
	agentNew      int
	hostRun       int
	processNew    int
	legacyNew     int
	commandKernel int
}

type constructionContract struct {
	agentNew      int
	hostRun       int
	processNew    int
	legacyNew     int
	commandKernel int
}

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
			if err != nil {
				t.Fatal(err)
			}
			if err := validateConstructionContract(
				counts,
				constructionContract{agentNew: 1, hostRun: 1},
			); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
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
			if err != nil {
				t.Fatal(err)
			}
			counts, err := inspectConstructionFiles(files)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateConstructionContract(
				counts,
				test.contract,
			); err != nil {
				t.Fatalf("%s: %v", test.path, err)
			}
		})
	}
}

func TestProductionCompositionConstructsOneSecretAuthoritySet(
	t *testing.T,
) {
	root := filepath.Clean(
		filepath.Join(jobmgrSourceRoot(t), "../../.."),
	)
	files, err := productionGoFiles(
		filepath.Join(
			root,
			"plugin/agent/jobmgr/composition",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
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
			got, err := countImportedCalls(
				files,
				call.importPath,
				call.function,
			)
			if err != nil {
				t.Fatal(err)
			}
			if got != call.want {
				t.Fatalf(
					"%s.%s calls=%d want=%d",
					call.importPath,
					call.function,
					got,
					call.want,
				)
			}
		})
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, forbidden := range []string{
			jobmgrImportPath + "/secretsctl",
			"secretstore.NewService(",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf(
					"active composition %s retains %q",
					path,
					forbidden,
				)
			}
		}
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
		"legacy manager beside process": {
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
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			counts, err := inspectConstructionSource(
				name+".go",
				test.source,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateConstructionContract(
				counts,
				test.contract,
			); err == nil {
				t.Fatal("adversarial construction source was accepted")
			}
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
				return 0, fmt.Errorf(
					"architecture guard: dot import %q",
					importPath,
				)
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
		total.legacyNew += counts.legacyNew
		total.commandKernel += counts.commandKernel
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
				return constructionCounts{}, fmt.Errorf(
					"architecture guard: dot import %q",
					importPath,
				)
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
				counts.legacyNew++
			case "NewCommandKernel":
				counts.commandKernel++
			}
		}
		return true
	})
	return counts, nil
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
		result = errors.Join(result, fmt.Errorf(
			"agent.New calls=%d want=%d",
			counts.agentNew,
			want.agentNew,
		))
	}
	if counts.hostRun != want.hostRun {
		result = errors.Join(result, fmt.Errorf(
			"agenthost.Run calls=%d want=%d",
			counts.hostRun,
			want.hostRun,
		))
	}
	if counts.processNew != want.processNew {
		result = errors.Join(result, fmt.Errorf(
			"composition.NewProcess calls=%d want=%d",
			counts.processNew,
			want.processNew,
		))
	}
	if counts.legacyNew != want.legacyNew {
		result = errors.Join(result, fmt.Errorf(
			"legacy jobmgr.New calls=%d want=%d",
			counts.legacyNew,
			want.legacyNew,
		))
	}
	if counts.commandKernel != want.commandKernel {
		result = errors.Join(result, fmt.Errorf(
			"jobmgr.NewCommandKernel calls=%d want=%d",
			counts.commandKernel,
			want.commandKernel,
		))
	}
	return result
}

func TestWorkPlanCannotCarryKernelLoopAbandonCallback(t *testing.T) {
	if _, found := reflect.TypeOf(jobmgr.WorkPlan{}).FieldByName("Abandon"); found {
		t.Fatal("WorkPlan exposes an opaque abandonment callback to KernelLoop")
	}
}

func TestCommandKernelUsesSourceSpecificPlanningPorts(t *testing.T) {
	constructor := reflect.TypeOf(jobmgr.NewCommandKernel)
	planner := reflect.TypeOf((*jobmgr.Planner)(nil)).Elem()
	functionCatalog := reflect.TypeOf((*jobmgr.FunctionCatalogPort)(nil)).Elem()
	if constructor.NumIn() < 2 ||
		constructor.In(constructor.NumIn()-2) != planner ||
		constructor.In(constructor.NumIn()-1) != functionCatalog {
		t.Fatalf("CommandKernel constructor does not end in Planner, FunctionCatalogPort: %v", constructor)
	}
	for index := 0; index < constructor.NumIn(); index++ {
		if constructor.In(index).Kind() == reflect.Map {
			t.Fatalf("CommandKernel constructor accepts generic source-indexed port map at argument %d", index)
		}
	}

	data, err := os.ReadFile(filepath.Join(jobmgrSourceRoot(t), "kernel.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{
		"map[lifecycle.Source]Planner",
		"planners[request.Source]",
		"preparePlan(request Request)",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("CommandKernel retains generic Function planner carrier %q", forbidden)
		}
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
