// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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
		path     string
		patterns []*regexp.Regexp
	}{
		{
			path: ".",
			patterns: []*regexp.Regexp{
				regexp.MustCompile(`\.(Attributes|Metrics)\b`),
				regexp.MustCompile(`(^|[^A-Za-z0-9_])(Attributes|Metrics)\s*[:\[]`),
			},
		},
		{
			path: "../../../../pkg/l2topology",
			patterns: []*regexp.Regexp{
				regexp.MustCompile(`\.Metrics\b`),
				regexp.MustCompile(`(^|[^A-Za-z0-9_])Metrics\s*[:\[]`),
			},
		},
		{
			path: "../../../../pkg/topology/graph",
			patterns: []*regexp.Regexp{
				regexp.MustCompile(`\.Metrics\b`),
				regexp.MustCompile(`(^|[^A-Za-z0-9_])Metrics\s*[:\[]`),
			},
		},
	} {
		assertNoForbiddenLinkMapCarrierUse(t, root.path, root.patterns)
	}
}

func assertNoForbiddenLinkMapCarrierUse(t *testing.T, root string, patterns []*regexp.Regexp) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		bs, err := os.ReadFile(path)
		require.NoError(t, err)
		for lineNo, line := range strings.Split(string(bs), "\n") {
			if isAllowedLinkMapCarrierLine(path, line) {
				continue
			}
			for _, pattern := range patterns {
				require.Falsef(t, pattern.MatchString(line), "forbidden link map carrier use in %s:%d: %s", path, lineNo+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	require.NoError(t, err)
}

func isAllowedLinkMapCarrierLine(path, line string) bool {
	if filepath.Base(path) == "func_topology_v1_presentation.go" && strings.Contains(line, "Metrics: map[string]string") {
		return true
	}
	return false
}
