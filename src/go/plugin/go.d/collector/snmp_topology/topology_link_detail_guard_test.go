// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestTopologyLinkDetailDoesNotUseMapCarriers(t *testing.T) {
	_, hasEndpointAttributes := reflect.TypeOf(graph.LinkEndpoint{}).FieldByName("Attributes")
	require.False(t, hasEndpointAttributes)
	_, hasLinkMetrics := reflect.TypeOf(graph.Link{}).FieldByName("Metrics")
	require.False(t, hasLinkMetrics)

	for _, root := range []struct {
		path  string
		names []string
	}{
		{
			path:  ".",
			names: []string{"Attributes", "Metrics"},
		},
		{
			path:  "../../../../pkg/l2topology",
			names: []string{"Metrics"},
		},
		{
			path:  "../../../../pkg/topology/graph",
			names: []string{"Metrics"},
		},
	} {
		assertNoForbiddenLinkMapCarrierUse(t, root.path, root.names...)
	}
}

func TestNoForbiddenLinkMapCarrierUseIgnoresCommentsAndStrings(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "fixture.go"), []byte(`package fixture

// Metrics: map[string]any{} in a comment is not code.
// Attributes["key"] in a comment is not code.
const metricsText = "Metrics[\"key\"]"
const attributesText = ".Attributes"
`), 0o644))

	assertNoForbiddenLinkMapCarrierUse(t, root, "Attributes", "Metrics")
}

func assertNoForbiddenLinkMapCarrierUse(t *testing.T, root string, names ...string) {
	t.Helper()
	forbidden := make(map[string]struct{}, len(names))
	for _, name := range names {
		forbidden[name] = struct{}{}
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)

		var scanErr error
		ast.Inspect(file, func(node ast.Node) bool {
			if scanErr != nil || node == nil {
				return false
			}
			switch typed := node.(type) {
			case *ast.SelectorExpr:
				scanErr = forbiddenLinkMapCarrierUseError(path, fset, typed, typed.Sel.Name, "selector", forbidden)
			case *ast.KeyValueExpr:
				scanErr = forbiddenLinkMapCarrierKeyValueError(path, fset, typed, forbidden)
			case *ast.IndexExpr:
				scanErr = forbiddenLinkMapCarrierIndexError(path, fset, typed.X, typed, forbidden)
			case *ast.IndexListExpr:
				scanErr = forbiddenLinkMapCarrierIndexError(path, fset, typed.X, typed, forbidden)
			}
			return scanErr == nil
		})
		if scanErr != nil {
			return scanErr
		}
		return nil
	})
	require.NoError(t, err)
}

func forbiddenLinkMapCarrierKeyValueError(
	path string,
	fset *token.FileSet,
	node *ast.KeyValueExpr,
	forbidden map[string]struct{},
) error {
	name := ""
	if ident, ok := node.Key.(*ast.Ident); ok {
		name = ident.Name
	}
	if name == "" {
		return nil
	}
	if isAllowedLinkMapCarrierUse(path, name, node) {
		return nil
	}
	return forbiddenLinkMapCarrierUseError(path, fset, node, name, "composite field", forbidden)
}

func forbiddenLinkMapCarrierIndexError(
	path string,
	fset *token.FileSet,
	expr ast.Expr,
	node ast.Node,
	forbidden map[string]struct{},
) error {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return nil
	}
	return forbiddenLinkMapCarrierUseError(path, fset, node, ident.Name, "index", forbidden)
}

func forbiddenLinkMapCarrierUseError(
	path string,
	fset *token.FileSet,
	node ast.Node,
	name string,
	kind string,
	forbidden map[string]struct{},
) error {
	if _, ok := forbidden[name]; !ok {
		return nil
	}
	position := fset.Position(node.Pos())
	return fmt.Errorf("forbidden link map carrier use in %s:%d: %s %q", path, position.Line, kind, name)
}

func isAllowedLinkMapCarrierUse(path, name string, node ast.Node) bool {
	if filepath.Base(path) != "func_topology_v1_presentation.go" || name != "Metrics" {
		return false
	}
	kv, ok := node.(*ast.KeyValueExpr)
	if !ok {
		return false
	}
	return isStringStringMapComposite(kv.Value)
}

func isStringStringMapComposite(expr ast.Expr) bool {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	mapType, ok := lit.Type.(*ast.MapType)
	if !ok {
		return false
	}
	key, ok := mapType.Key.(*ast.Ident)
	if !ok || key.Name != "string" {
		return false
	}
	value, ok := mapType.Value.(*ast.Ident)
	return ok && value.Name == "string"
}
