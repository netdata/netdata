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
// keeps these eager while deferring only the chart template).
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
