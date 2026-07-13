// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

// TestStockProfiles_OptionalBlocksUncommentValid guards the commented "optional
// metrics" blocks shipped in every stock profile. With every optional block
// uncommented (what an operator does by hand to enable a metric + its chart),
// each profile must still load, validate, and have every chart selector resolve
// to a declared metric series. The decoder is intentionally non-strict, so this
// test — not the decoder — is what keeps the optional blocks honest.
func TestStockProfiles_OptionalBlocksUncommentValid(t *testing.T) {
	stockDir := cwProfilesDirFromThisFile()
	require.NotEmpty(t, stockDir)

	entries, err := os.ReadDir(stockDir)
	require.NoError(t, err)

	tmp := t.TempDir()
	want := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := strings.ToLower(filepath.Ext(e.Name())); ext != ".yaml" && ext != ".yml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(stockDir, e.Name()))
		require.NoError(t, err)
		uncommented := uncommentOptionalBlocks(string(raw))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, e.Name()), []byte(uncommented), 0o600))
		want++
	}
	require.NotZero(t, want)

	// Loading as stock validates every uncommented profile fatally.
	catalog, err := loadFromDirs([]profilecatalog.DirSpec{{Path: tmp, IsStock: true}})
	require.NoError(t, err, "a stock profile is invalid once its optional blocks are uncommented")
	require.Len(t, catalog.AllProfiles(), want)

	// Two-way coverage between declared metric series and chart selectors, in the
	// fully-uncommented state:
	//   (1) every chart selector resolves to a declared metric series (no dangling
	//       selector); Normalize leaves an unresolved shorthand un-qualified, so it
	//       will not be a key in the visible (fully-qualified) series set; and
	//   (2) every declared metric series is targeted by at least one chart dimension.
	//       An un-charted metric would still bill GetMetricData yet render nothing, so
	//       every optional metric must ship a paired chart (the A1 pattern).
	for _, rp := range catalog.AllProfiles() {
		visible := visibleSeriesForProfile(rp.Name, rp.Config.Metrics)
		selected := make(map[string]struct{})
		for _, sel := range chartSelectors(rp.Config.Template) {
			series, _, ok := splitSelectorSeries(sel)
			if !assert.Truef(t, ok, "%s: unparseable chart selector %q", rp.Name, sel) {
				continue
			}
			selected[series] = struct{}{}
			_, found := visible[series]
			assert.Truef(t, found, "%s: chart selector %q does not resolve to a declared metric series", rp.Name, sel)
		}
		for series := range visible {
			_, charted := selected[series]
			assert.Truef(t, charted, "%s: metric series %q has no chart dimension (would bill GetMetricData but render nothing)", rp.Name, series)
		}
	}
}

// uncommentOptionalBlocks strips the leading "# " from commented YAML structural
// lines (the optional metric/chart blocks), mirroring what an operator does by
// hand. Prose comment lines (section headers, docs pointers) do not start with a
// structural key and stay commented.
func uncommentOptionalBlocks(raw string) string {
	keys := []string{
		"- id:", "metric_name:", "statistics:", "query:", "period:", "lookback:", "publication_delay:", "rate:",
		"context:", "title:", "family:", "units:", "algorithm:", "dimensions:",
		"- selector:", "selector:", "name:",
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		trimmed := strings.TrimLeft(ln, " ")
		uncommented := ""
		if strings.HasPrefix(trimmed, "# ") {
			content := trimmed[len("# "):]
			c := strings.TrimLeft(content, " ")
			for _, k := range keys {
				if strings.HasPrefix(c, k) {
					uncommented = ln[:len(ln)-len(trimmed)] + content
					break
				}
			}
		}
		if uncommented != "" {
			out = append(out, uncommented)
		} else {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// chartSelectors returns every chart-dimension selector in a group, recursively.
func chartSelectors(group charttpl.Group) []string {
	var out []string
	for _, c := range group.Charts {
		for _, d := range c.Dimensions {
			out = append(out, d.Selector)
		}
	}
	for _, g := range group.Groups {
		out = append(out, chartSelectors(g)...)
	}
	return out
}
