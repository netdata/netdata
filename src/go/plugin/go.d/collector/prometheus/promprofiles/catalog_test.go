// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Generic loader mechanics (directory walk, .yaml/.yml filter, _-prefix skip,
// basename validation, stock/user override precedence, duplicate handling,
// missing-directory handling, empty catalogs) are owned and tested by
// pkg/profilecatalog. This file covers only Prometheus-specific behavior: strict
// header decoding, header validation, lazy template hydration, and
// case-insensitive resolution.

type fileSpec struct {
	stock   bool
	name    string
	content string
}

// profileYAML is a valid profile body: no name (identity is the filename) and an
// explicit metrics: list, exactly like any v2 chart template.
func profileYAML(match string) string {
	return fmt.Sprintf(`match: "%s"
template:
  family: test
  metrics:
    - test_metric_total
  charts:
    - title: Test Metric
      context: test_metric
      units: count
      dimensions:
        - selector: test_metric_total
          name: total
`, match)
}

func profileYAMLWithAutogenSelector(match string, allow, deny []string) string {
	var selector strings.Builder
	if allow != nil {
		if len(allow) == 0 {
			selector.WriteString("    allow: []\n")
		} else {
			selector.WriteString("    allow:\n")
			for _, item := range allow {
				fmt.Fprintf(&selector, "      - %q\n", item)
			}
		}
	}
	if deny != nil {
		if len(deny) == 0 {
			selector.WriteString("    deny: []\n")
		} else {
			selector.WriteString("    deny:\n")
			for _, item := range deny {
				fmt.Fprintf(&selector, "      - %q\n", item)
			}
		}
	}
	return fmt.Sprintf(`match: "%s"
autogen:
  selector:
%stemplate:
  family: test
  metrics:
    - test_metric_total
  charts:
    - title: Test Metric
      context: test_metric
      units: count
      dimensions:
        - selector: test_metric_total
          name: total
`, match, selector.String())
}

// profileYAMLNoChart has a valid header but a structurally invalid template (a
// group with no chart), so it decodes fine but fails template hydration.
func profileYAMLNoChart(match string) string {
	return fmt.Sprintf(`match: "%s"
template:
  family: test
`, match)
}

func profileYAMLWithRelabeling(match, relabeling string) string {
	return strings.Replace(profileYAML(match), "template:\n", "relabeling:\n"+relabeling+"template:\n", 1)
}

func profileYAMLWithFallbackType(match, fallbackType string) string {
	return strings.Replace(profileYAML(match), "template:\n", "fallback_type:"+fallbackType+"\ntemplate:\n", 1)
}

const validFallbackType = `
  gauge:
    - app_value
  counter:
    - app_requests
`

const validRelabeling = `  - match: "app_*"
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: "app_(.*)"
        target_label: __name__
        replacement: "app_normalized_${1}"
        action: replace
`

func loadCatalog(t *testing.T, files ...fileSpec) (Catalog, error) {
	t.Helper()

	userDir := t.TempDir()
	stockDir := t.TempDir()
	for _, f := range files {
		dir := userDir
		if f.stock {
			dir = stockDir
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0o600))
	}
	// User dir first mirrors defaultDirSpecs ordering.
	return LoadFromDirs([]DirSpec{{Path: userDir, IsStock: false}, {Path: stockDir, IsStock: true}})
}

// TestLoadFromDirs_headerDecodeAndValidation covers the strict header decode and
// header-level validation that Prometheus applies at load time (the lazy design
// keeps these eager while deferring profile-owned policies and the chart template).
func TestLoadFromDirs_headerDecodeAndValidation(t *testing.T) {
	tests := map[string]struct {
		content string
		wantErr bool
	}{
		"valid stock profile": {
			content: profileYAML("app_*"),
		},
		"strict yaml rejects unknown top-level field": {
			content: profileYAML("app_*") + "bogus: 1\n",
			wantErr: true,
		},
		"stray name field is rejected": {
			content: "name: app\n" + profileYAML("app_*"),
			wantErr: true,
		},
		"empty match is fatal": {
			content: profileYAML(""),
			wantErr: true,
		},
		"valid autogen selector is accepted eagerly and by lazy template decode": {
			content: profileYAMLWithAutogenSelector(
				"app_*",
				[]string{`app_metric{region=~"west|east"}`},
				[]string{`app_metric{environment="dev"}`},
			),
		},
		"empty autogen object is fatal": {
			content: strings.Replace(profileYAML("app_*"), "template:\n", "autogen: {}\ntemplate:\n", 1),
			wantErr: true,
		},
		"null autogen selector is fatal": {
			content: strings.Replace(profileYAML("app_*"), "template:\n", "autogen:\n  selector: null\ntemplate:\n", 1),
			wantErr: true,
		},
		"empty autogen selector is fatal": {
			content: profileYAMLWithAutogenSelector("app_*", nil, nil),
			wantErr: true,
		},
		"explicit empty selector lists are fatal": {
			content: profileYAMLWithAutogenSelector("app_*", []string{}, []string{}),
			wantErr: true,
		},
		"strict yaml rejects unknown autogen field": {
			content: strings.Replace(
				profileYAMLWithAutogenSelector("app_*", nil, []string{"metric*"}),
				"  selector:",
				"  unknown:",
				1,
			),
			wantErr: true,
		},
		"whitespace-only autogen selector entry is fatal": {
			content: profileYAMLWithAutogenSelector("app_*", nil, []string{"  "}),
			wantErr: true,
		},
		"invalid autogen selector is fatal": {
			content: profileYAMLWithAutogenSelector("app_*", []string{`metric{region="west",}`}, nil),
			wantErr: true,
		},
		"valid allow-only selector is accepted": {
			content: profileYAMLWithAutogenSelector("app_*", []string{"app_*"}, nil),
		},
		"valid deny-only selector is accepted": {
			content: profileYAMLWithAutogenSelector("app_*", nil, []string{"app_*"}),
		},
		"invalid selector after a valid entry is fatal": {
			content: profileYAMLWithAutogenSelector("app_*", []string{"app_*", `metric{region="west",}`}, nil),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadCatalog(t, fileSpec{stock: true, name: "app.yaml", content: tc.content})
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProfileTemplateHydrationPreservesCrossFieldYAMLAnchors(t *testing.T) {
	content := `match: &metric app_metric
template:
  family: test
  metrics: [*metric]
  charts:
    - title: Test Metric
      context: test_metric
      units: count
      dimensions:
        - selector: *metric
          name: value
`

	cat, err := loadCatalog(t, fileSpec{name: "app.yaml", content: content})
	require.NoError(t, err)
	profile, ok := cat.Get("app")
	require.True(t, ok)
	_, err = profile.Template()
	require.NoError(t, err)
}

func TestProfileRelabelHydrationPreservesCrossFieldYAMLAnchors(t *testing.T) {
	content := `match: app_*
app: &target app_normalized
relabeling:
  - match: app_raw
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_raw
        target_label: __name__
        replacement: *target
template:
  family: test
  metrics: [app_normalized]
  charts:
    - title: Test Metric
      context: test_metric
      units: count
      dimensions:
        - selector: app_normalized
          name: value
`

	cat, err := loadCatalog(t, fileSpec{name: "app.yaml", content: content})
	require.NoError(t, err)
	profile, ok := cat.Get("app")
	require.True(t, ok)
	blocks, err := profile.Relabeling()
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Len(t, blocks[0].MetricRelabelConfigs, 1)
	assert.Equal(t, "app_normalized", blocks[0].MetricRelabelConfigs[0].Replacement)
}

func TestProfileFallbackHydrationPreservesCrossFieldYAMLAnchors(t *testing.T) {
	content := `match: &metric app_value
fallback_type:
  gauge: [*metric]
template:
  family: test
  metrics: [*metric]
  charts:
    - title: Test Metric
      context: test_metric
      units: count
      dimensions:
        - selector: *metric
          name: value
`

	cat, err := loadCatalog(t, fileSpec{name: "app.yaml", content: content})
	require.NoError(t, err)
	profile, ok := cat.Get("app")
	require.True(t, ok)
	fallbackType, err := profile.FallbackType()
	require.NoError(t, err)
	assert.Equal(t, []string{"app_value"}, fallbackType.Gauge)
}

func TestLoadFromDirs_stockFallbackTypeHydratesLazily(t *testing.T) {
	tests := map[string]struct {
		fallbackType string
		want         FallbackType
		wantErr      string
	}{
		"valid": {
			fallbackType: validFallbackType,
			want: FallbackType{
				Gauge:   []string{"app_value"},
				Counter: []string{"app_requests"},
			},
		},
		"null": {
			fallbackType: " null",
			wantErr:      "'fallback_type' must not be empty",
		},
		"empty object": {
			fallbackType: " {}",
			wantErr:      "'fallback_type' must contain at least one gauge or counter pattern",
		},
		"empty lists": {
			fallbackType: "\n  gauge: []\n  counter: []",
			wantErr:      "'fallback_type' must contain at least one gauge or counter pattern",
		},
		"unknown field": {
			fallbackType: "\n  unknown: true",
			wantErr:      "field unknown not found",
		},
		"blank gauge pattern": {
			fallbackType: "\n  gauge: ['   ']",
			wantErr:      "'fallback_type.gauge[0]' must not be empty",
		},
		"padded gauge pattern": {
			fallbackType: "\n  gauge: [' app_value ']",
			wantErr:      "'fallback_type.gauge[0]' must not have leading or trailing whitespace",
		},
		"invalid counter pattern": {
			fallbackType: "\n  counter: ['[']",
			wantErr:      "'fallback_type.counter[0]'",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cat, err := loadCatalog(t, fileSpec{
				stock:   true,
				name:    "app.yaml",
				content: profileYAMLWithFallbackType("app_*", tc.fallbackType),
			})
			require.NoError(t, err, "stock fallback policy must remain lazy")

			p, ok := cat.Get("app")
			require.True(t, ok)
			assert.True(t, p.HasFallbackType(), "a present fallback_type key must be visible without hydration")

			got, err1 := p.FallbackType()
			if tc.wantErr != "" {
				require.ErrorContains(t, err1, tc.wantErr)
				_, err2 := p.FallbackType()
				assert.Equal(t, err1, err2, "the hydration error must be memoized")
				return
			}
			require.NoError(t, err1)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestProfile_FallbackTypeAbsent(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{stock: true, name: "app.yaml", content: profileYAML("app_*")})
	require.NoError(t, err)
	p, ok := cat.Get("app")
	require.True(t, ok)

	assert.False(t, p.HasFallbackType())
	got, err := p.FallbackType()
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestLoadFromDirs_userBadFallbackTypeSkippedStockSurvives(t *testing.T) {
	cat, err := loadCatalog(t,
		fileSpec{stock: true, name: "app.yaml", content: profileYAML("stock_*")},
		fileSpec{stock: false, name: "app.yaml", content: profileYAMLWithFallbackType("user_*", " {}")},
	)
	require.NoError(t, err)

	p, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, "stock_*", p.Match)
	assert.False(t, p.HasFallbackType())
}

func TestProfile_FallbackTypeReturnsIndependentCopies(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{
		stock:   true,
		name:    "app.yaml",
		content: profileYAMLWithFallbackType("app_*", validFallbackType),
	})
	require.NoError(t, err)

	assertIndependent := func(t *testing.T, p Profile) {
		t.Helper()
		first, err := p.FallbackType()
		require.NoError(t, err)
		first.Gauge[0] = "changed"
		first.Counter[0] = "changed"

		second, err := p.FallbackType()
		require.NoError(t, err)
		assert.Equal(t, []string{"app_value"}, second.Gauge)
		assert.Equal(t, []string{"app_requests"}, second.Counter)
	}

	fromGet, ok := cat.Get("app")
	require.True(t, ok)
	assertIndependent(t, fromGet)

	ordered := cat.OrderedProfiles()
	require.Len(t, ordered, 1)
	assertIndependent(t, ordered[0])

	resolved, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	assertIndependent(t, resolved[0])
}

func TestProfile_FallbackTypeConcurrent(t *testing.T) {
	for name, fallbackType := range map[string]string{
		"success": validFallbackType,
		"error":   "\n  gauge: ['[']",
	} {
		t.Run(name, func(t *testing.T) {
			cat, err := loadCatalog(t, fileSpec{
				stock:   true,
				name:    "app.yaml",
				content: profileYAMLWithFallbackType("app_*", fallbackType),
			})
			require.NoError(t, err)
			p, ok := cat.Get("app")
			require.True(t, ok)

			var wg sync.WaitGroup
			for range 16 {
				wg.Go(func() {
					got, err := p.FallbackType()
					if name == "error" {
						assert.Error(t, err)
						assert.Empty(t, got)
					} else {
						assert.NoError(t, err)
						assert.Equal(t, []string{"app_value"}, got.Gauge)
					}
				})
			}
			wg.Wait()
		})
	}
}

func TestProfileAutogenSelectorOwnership(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{
		stock: true,
		name:  "app.yaml",
		content: profileYAMLWithAutogenSelector(
			"app_*",
			[]string{"app_*", `app_metric{region="west"}`},
			[]string{`app_metric{environment="dev"}`, "μέτρο*"},
		),
	})
	require.NoError(t, err)
	want := &metrixselector.Expr{
		Allow: []string{"app_*", `app_metric{region="west"}`},
		Deny:  []string{`app_metric{environment="dev"}`, "μέτρο*"},
	}

	fromGet, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, want, fromGet.AutogenSelector())
	accessorCopy := fromGet.AutogenSelector()
	accessorCopy.Allow[0] = "accessor_mutation"
	accessorCopy.Deny[0] = "accessor_mutation"
	assert.Equal(t, want, fromGet.AutogenSelector())

	fromGet.autogenSelector.Allow[0] = "get_mutation"
	fromGet.autogenSelector.Deny[0] = "get_mutation"
	afterGetMutation, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, want, afterGetMutation.AutogenSelector())

	ordered := cat.OrderedProfiles()
	require.Len(t, ordered, 1)
	ordered[0].autogenSelector.Allow[0] = "ordered_mutation"
	ordered[0].autogenSelector.Deny[0] = "ordered_mutation"
	afterOrderedMutation, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, want, afterOrderedMutation.AutogenSelector())

	resolved, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	resolved[0].autogenSelector.Allow[0] = "resolve_mutation"
	resolved[0].autogenSelector.Deny[0] = "resolve_mutation"
	afterResolveMutation, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, want, afterResolveMutation.AutogenSelector())

	template, err := afterResolveMutation.Template()
	require.NoError(t, err)
	assert.NotEmpty(t, template.Charts)
}

// TestLoadFromDirs_stockTemplateHydratesLazily verifies a stock profile with a
// valid header but a structurally invalid template LOADS successfully (matching
// only needs the header) and errors only when its template is hydrated.
func TestLoadFromDirs_stockTemplateHydratesLazily(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{stock: true, name: "app.yaml", content: profileYAMLNoChart("app_*")})
	require.NoError(t, err, "a stock profile with a bad template must still load")

	got, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	require.Len(t, got, 1)

	_, err1 := got[0].Template()
	assert.Error(t, err1, "template hydration must surface the invalid template")

	// The memoized error is returned again (hydration runs once).
	_, err2 := got[0].Template()
	assert.Equal(t, err1, err2)
}

// TestLoadFromDirs_userBadTemplateSkippedStockSurvives verifies O3: a user
// profile validates its template at load, so a broken user override is skipped
// and the stock profile of the same name survives.
func TestLoadFromDirs_userBadTemplateSkippedStockSurvives(t *testing.T) {
	cat, err := loadCatalog(t,
		fileSpec{stock: true, name: "app.yaml", content: profileYAML("stock_*")},
		fileSpec{stock: false, name: "app.yaml", content: profileYAMLNoChart("user_*")},
	)
	require.NoError(t, err)

	got, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "stock_*", got[0].Match, "the valid stock profile must survive the broken user override")

	tmpl, err := got[0].Template()
	require.NoError(t, err, "the surviving stock template must hydrate")
	assert.NotEmpty(t, tmpl.Charts)
}

func TestLoadFromDirs_stockRelabelingHydratesLazily(t *testing.T) {
	tests := map[string]struct {
		relabeling string
		wantBlocks int
		wantErr    string
	}{
		"valid": {
			relabeling: validRelabeling,
			wantBlocks: 1,
		},
		"null": {
			relabeling: "  null\n",
			wantErr:    "'relabeling' must not be empty",
		},
		"empty": {
			relabeling: "  []\n",
			wantErr:    "'relabeling' must not be empty",
		},
		"invalid block field": {
			relabeling: strings.Replace(validRelabeling, "    metric_relabel_configs:", "    unknown: true\n    metric_relabel_configs:", 1),
			wantErr:    "field unknown not found",
		},
		"invalid rule field": {
			relabeling: strings.Replace(validRelabeling, "        action: replace", "        unknown: true\n        action: replace", 1),
			wantErr:    "field unknown not found",
		},
		"invalid rule": {
			relabeling: strings.Replace(validRelabeling, "action: replace", "action: bogus", 1),
			wantErr:    "unknown relabel action",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cat, err := loadCatalog(t, fileSpec{
				stock:   true,
				name:    "app.yaml",
				content: profileYAMLWithRelabeling("app_*", tc.relabeling),
			})
			require.NoError(t, err, "stock relabeling must remain lazy")

			p, ok := cat.Get("app")
			require.True(t, ok)
			assert.True(t, p.HasRelabeling(), "a present relabeling key must be visible without hydration")

			blocks, err1 := p.Relabeling()
			if tc.wantErr != "" {
				require.ErrorContains(t, err1, tc.wantErr)
				_, err2 := p.Relabeling()
				assert.Equal(t, err1, err2, "the hydration error must be memoized")
				return
			}
			require.NoError(t, err1)
			assert.Len(t, blocks, tc.wantBlocks)

			tmpl, err := p.Template()
			require.NoError(t, err, "relabeling must not break template hydration")
			assert.NotEmpty(t, tmpl.Charts)
		})
	}
}

func TestProfile_RelabelingAbsent(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{stock: true, name: "app.yaml", content: profileYAML("app_*")})
	require.NoError(t, err)
	p, ok := cat.Get("app")
	require.True(t, ok)

	assert.False(t, p.HasRelabeling())
	blocks, err := p.Relabeling()
	require.NoError(t, err)
	assert.Nil(t, blocks)
}

func TestLoadFromDirs_userBadRelabelingSkippedStockSurvives(t *testing.T) {
	bad := strings.Replace(validRelabeling, "action: replace", "action: bogus", 1)
	cat, err := loadCatalog(t,
		fileSpec{stock: true, name: "app.yaml", content: profileYAML("stock_*")},
		fileSpec{stock: false, name: "app.yaml", content: profileYAMLWithRelabeling("user_*", bad)},
	)
	require.NoError(t, err)

	p, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, "stock_*", p.Match)
	assert.False(t, p.HasRelabeling())
}

func TestProfile_RelabelingReturnsIndependentCopies(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{
		stock:   true,
		name:    "app.yaml",
		content: profileYAMLWithRelabeling("app_*", validRelabeling),
	})
	require.NoError(t, err)

	assertIndependent := func(t *testing.T, p Profile) {
		t.Helper()
		first, err := p.Relabeling()
		require.NoError(t, err)
		require.Len(t, first, 1)
		require.Len(t, first[0].MetricRelabelConfigs, 1)
		firstRegex := first[0].MetricRelabelConfigs[0].Regex.Regexp
		first[0].Match = "changed_*"
		first[0].MetricRelabelConfigs[0].SourceLabels[0] = "changed"
		firstRegex.Longest()

		second, err := p.Relabeling()
		require.NoError(t, err)
		require.Len(t, second, 1)
		assert.Equal(t, "app_*", second[0].Match)
		assert.Equal(t, []string{"__name__"}, second[0].MetricRelabelConfigs[0].SourceLabels)
		assert.NotSame(t, firstRegex, second[0].MetricRelabelConfigs[0].Regex.Regexp)
	}

	fromGet, ok := cat.Get("app")
	require.True(t, ok)
	assertIndependent(t, fromGet)

	ordered := cat.OrderedProfiles()
	require.Len(t, ordered, 1)
	assertIndependent(t, ordered[0])

	resolved, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	assertIndependent(t, resolved[0])
}

func TestProfile_RelabelingConcurrent(t *testing.T) {
	for name, relabeling := range map[string]string{
		"success": validRelabeling,
		"error":   strings.Replace(validRelabeling, "action: replace", "action: bogus", 1),
	} {
		t.Run(name, func(t *testing.T) {
			cat, err := loadCatalog(t, fileSpec{
				stock:   true,
				name:    "app.yaml",
				content: profileYAMLWithRelabeling("app_*", relabeling),
			})
			require.NoError(t, err)
			p, ok := cat.Get("app")
			require.True(t, ok)

			var wg sync.WaitGroup
			for range 16 {
				wg.Go(func() {
					blocks, err := p.Relabeling()
					if name == "error" {
						assert.Error(t, err)
						assert.Nil(t, blocks)
					} else {
						assert.NoError(t, err)
						assert.Len(t, blocks, 1)
					}
				})
			}
			wg.Wait()
		})
	}
}

// TestProfile_TemplateConcurrent exercises concurrent hydration of a shared
// catalog profile (run with -race).
func TestProfile_TemplateConcurrent(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{stock: true, name: "app.yaml", content: profileYAML("app_*")})
	require.NoError(t, err)
	got, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	p := got[0]

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			tmpl, err := p.Template()
			assert.NoError(t, err)
			assert.NotEmpty(t, tmpl.Charts)
		})
	}
	wg.Wait()
}

// TestProfile_TemplateReturnsIndependentCopies verifies Template() hands out an
// independent deep copy each call, so a caller mutating the result cannot corrupt
// the process-wide catalog (a shared, cached template).
func TestProfile_TemplateReturnsIndependentCopies(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{stock: true, name: "app.yaml", content: profileYAML("app_*")})
	require.NoError(t, err)
	got, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	p := got[0]

	t1, err := p.Template()
	require.NoError(t, err)
	require.NotEmpty(t, t1.Charts)
	t1.Charts[0].Title = "MUTATED"

	t2, err := p.Template()
	require.NoError(t, err)
	require.NotEmpty(t, t2.Charts)
	assert.NotEqual(t, "MUTATED", t2.Charts[0].Title, "Template() must return independent copies")
}

func TestCatalog_Resolve(t *testing.T) {
	cat, err := loadCatalog(t, fileSpec{stock: true, name: "app.yaml", content: profileYAML("a_*")})
	require.NoError(t, err)

	t.Run("matches case-insensitively", func(t *testing.T) {
		got, err := cat.Resolve([]string{"App"})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "app", got[0].Name)
	})
	t.Run("unknown name errors", func(t *testing.T) {
		_, err := cat.Resolve([]string{"nope"})
		assert.Error(t, err)
	})
	t.Run("empty selection errors", func(t *testing.T) {
		_, err := cat.Resolve(nil)
		assert.Error(t, err)
	})
}
