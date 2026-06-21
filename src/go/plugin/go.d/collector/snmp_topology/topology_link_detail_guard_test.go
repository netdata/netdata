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
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestTopologyLinkDetailDoesNotUseMapCarriers(t *testing.T) {
	_, hasActorAttributes := reflect.TypeFor[graph.Actor]().FieldByName("Attributes")
	require.False(t, hasActorAttributes)
	_, hasActorTables := reflect.TypeFor[graph.Actor]().FieldByName("Tables")
	require.False(t, hasActorTables)
	_, hasEndpointAttributes := reflect.TypeFor[graph.LinkEndpoint]().FieldByName("Attributes")
	require.False(t, hasEndpointAttributes)
	_, hasLinkMetrics := reflect.TypeFor[graph.Link]().FieldByName("Metrics")
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
			names: []string{"Attributes", "Tables", "Metrics"},
		},
		{
			path:  "../../../../pkg/topology/graph",
			names: []string{"Attributes", "Tables", "Metrics"},
		},
	} {
		assertNoForbiddenLinkMapCarrierUse(t, root.path, root.names...)
	}
}

func TestTopologyActorSemanticDetailDoesNotFallbackToLabels(t *testing.T) {
	forbiddenKeys := map[string]struct{}{
		"display_name":   {},
		"display_source": {},
		"inferred":       {},
		"name":           {},
		tagOSPFRouterID:  {},
	}
	for _, root := range []struct {
		path  string
		names []string
	}{
		{
			path:  ".",
			names: []string{"actor", "left", "right", "base", "extra", "src", "dst"},
		},
		{
			path:  "../../../../pkg/l2topology",
			names: []string{"actor", "left", "right", "base", "extra", "merged"},
		},
	} {
		assertNoForbiddenTopologyActorLabelFallbacks(t, root.path, topologyActorLabelFallbackRootNames(root.names...), forbiddenKeys)
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

func TestTopologyActorSemanticDetailGuardCatchesIndexedActorRoots(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "fixture.go"), []byte(`package fixture

func fallback(actors []struct{ Actor struct{ Labels map[string]string } }) string {
	return actors[0].Actor.Labels["display_name"]
}
`), 0o644))

	err := forbiddenTopologyActorLabelFallbacksError(
		root,
		topologyActorLabelFallbackRootNames("actors"),
		map[string]struct{}{"display_name": {}},
	)
	require.Error(t, err)
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

func assertNoForbiddenTopologyActorLabelFallbacks(
	t *testing.T,
	root string,
	rootNames map[string]struct{},
	forbiddenKeys map[string]struct{},
) {
	t.Helper()
	require.NoError(t, forbiddenTopologyActorLabelFallbacksError(root, rootNames, forbiddenKeys))
}

func forbiddenTopologyActorLabelFallbacksError(
	root string,
	rootNames map[string]struct{},
	forbiddenKeys map[string]struct{},
) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		var scanErr error
		ast.Inspect(file, func(node ast.Node) bool {
			if scanErr != nil || node == nil {
				return false
			}
			index, ok := node.(*ast.IndexExpr)
			if !ok {
				return true
			}
			selector, ok := index.X.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Labels" {
				return true
			}
			if _, ok := rootNames[topologyLabelFallbackRootName(selector.X)]; !ok {
				return true
			}
			key := topologyLabelFallbackIndexKey(index.Index)
			if _, forbidden := forbiddenKeys[key]; !forbidden {
				return true
			}
			position := fset.Position(index.Pos())
			scanErr = fmt.Errorf("forbidden topology actor label fallback in %s:%d: %q", path, position.Line, key)
			return false
		})
		return scanErr
	})
}

func topologyActorLabelFallbackRootNames(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}

func topologyLabelFallbackRootName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return topologyLabelFallbackRootName(typed.X)
	case *ast.IndexExpr:
		return topologyLabelFallbackRootName(typed.X)
	case *ast.IndexListExpr:
		return topologyLabelFallbackRootName(typed.X)
	case *ast.ParenExpr:
		return topologyLabelFallbackRootName(typed.X)
	default:
		return ""
	}
}

func topologyLabelFallbackIndexKey(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.BasicLit:
		if typed.Kind != token.STRING {
			return ""
		}
		value, err := strconv.Unquote(typed.Value)
		if err != nil {
			return ""
		}
		return value
	case *ast.Ident:
		return typed.Name
	default:
		return ""
	}
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
