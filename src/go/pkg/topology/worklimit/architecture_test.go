// SPDX-License-Identifier: GPL-3.0-or-later

package worklimit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoundedTopologyPackagesDoNotBypassWorkLimitedSorts(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	directories := []string{
		"pkg/l2topology/internal/pipeline",
		"pkg/l2topology/internal/projector",
		"plugin/go.d/collector/snmp_topology/internal/topologyenrich",
		"plugin/go.d/collector/snmp_topology/internal/topologymodel",
		"plugin/go.d/collector/snmp_topology/internal/topologyoptions",
		"plugin/go.d/collector/snmp_topology/internal/topologyshape",
		"plugin/go.d/collector/snmp_topology/internal/topologyv1",
	}

	for _, directory := range directories {
		directory := filepath.Join(moduleRoot, directory)
		err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
			require.NoError(t, err)
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			source, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			require.NoError(t, err)
			for _, call := range rawDynamicSortCalls(source) {
				t.Errorf("%s calls %s directly; bounded topology packages must route dynamic sorts through worklimit", path, call)
			}
			return nil
		})
		require.NoError(t, err)
	}
}

func TestRawDynamicSortCallsRecognizesAliases(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package fixture
import (
  ordered "sort"
  "slices"
)
func run(values []string) {
  ordered.Strings(values)
  slices.Sort(values)
}`, parser.SkipObjectResolution)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"sort.Strings", "slices.Sort"}, rawDynamicSortCalls(source))
}

func TestPipelineStringOrderingDoesNotUseCountOnlySortHelpers(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "l2topology", "internal", "pipeline", "sorting.go"))
	source, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	var countOnly []string
	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "SortSlice" && selector.Sel.Name != "SortSliceStable" {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && identifier.Name == "worklimit" {
			countOnly = append(countOnly, selector.Sel.Name)
		}
		return true
	})
	require.Empty(t, countOnly, "pipeline string ordering must use string-aware worklimit helpers")
}

func rawDynamicSortCalls(file *ast.File) []string {
	if file == nil {
		return nil
	}
	aliases := make(map[string]string)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != "sort" && path != "slices" {
			continue
		}
		name := path
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = path
	}

	forbidden := map[string]map[string]struct{}{
		"sort": {
			"Float64s": {}, "Ints": {}, "Slice": {}, "SliceStable": {}, "Sort": {}, "Stable": {}, "Strings": {},
		},
		"slices": {
			"Sort": {}, "SortFunc": {}, "SortStableFunc": {},
		},
	}
	var calls []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		path := aliases[identifier.Name]
		if _, banned := forbidden[path][selector.Sel.Name]; banned {
			calls = append(calls, path+"."+selector.Sel.Name)
		}
		return true
	})
	return calls
}
