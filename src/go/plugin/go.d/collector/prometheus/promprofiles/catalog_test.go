// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestLoadFromDirs(t *testing.T) {
	tests := map[string]struct {
		files     []fileSpec
		wantErr   bool
		wantNames []string
	}{
		"single stock profile": {
			files:     []fileSpec{{stock: true, name: "app.yaml", content: profileYAML("app_*")}},
			wantNames: []string{"app"},
		},
		"ignores non-yaml and underscore-prefixed files": {
			files: []fileSpec{
				{stock: true, name: "app.yaml", content: profileYAML("app_*")},
				{stock: true, name: "_partial.yaml", content: profileYAML("p_*")},
				{stock: true, name: "notes.txt", content: "ignored"},
			},
			wantNames: []string{"app"},
		},
		"strict yaml rejects unknown field in stock profile": {
			files:   []fileSpec{{stock: true, name: "app.yaml", content: profileYAML("app_*") + "bogus: 1\n"}},
			wantErr: true,
		},
		"stray name field is rejected in stock profile": {
			files:   []fileSpec{{stock: true, name: "app.yaml", content: "name: app\n" + profileYAML("app_*")}},
			wantErr: true,
		},
		"empty match in stock profile is fatal": {
			files:   []fileSpec{{stock: true, name: "app.yaml", content: profileYAML("")}},
			wantErr: true,
		},
		"invalid basename in stock profile is fatal": {
			files:   []fileSpec{{stock: true, name: "App.yaml", content: profileYAML("app_*")}},
			wantErr: true,
		},
		"invalid profile in user dir is skipped": {
			files: []fileSpec{
				{stock: true, name: "good.yaml", content: profileYAML("g_*")},
				{stock: false, name: "bad.yaml", content: profileYAML("")},
			},
			wantNames: []string{"good"},
		},
		"invalid basename in user dir is skipped": {
			files: []fileSpec{
				{stock: true, name: "good.yaml", content: profileYAML("g_*")},
				{stock: false, name: "Bad.yaml", content: profileYAML("b_*")},
			},
			wantNames: []string{"good"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cat, err := loadCatalog(t, tc.files...)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			var got []string
			for _, p := range cat.OrderedProfiles() {
				got = append(got, p.Name)
			}
			assert.ElementsMatch(t, tc.wantNames, got)
		})
	}
}

func TestLoadFromDirs_userOverridesStock(t *testing.T) {
	cat, err := loadCatalog(t,
		fileSpec{stock: true, name: "app.yaml", content: profileYAML("stock_*")},
		fileSpec{stock: false, name: "app.yaml", content: profileYAML("user_*")},
	)
	require.NoError(t, err)

	got, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "user_*", got[0].Match)
}

func TestLoadFromDirs_userOverridesStockWhenStockSeenFirst(t *testing.T) {
	userDir := t.TempDir()
	stockDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stockDir, "app.yaml"), []byte(profileYAML("stock_*")), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "app.yaml"), []byte(profileYAML("user_*")), 0o600))

	cat, err := LoadFromDirs([]DirSpec{{Path: stockDir, IsStock: true}, {Path: userDir, IsStock: false}})
	require.NoError(t, err)

	got, err := cat.Resolve([]string{"app"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "user_*", got[0].Match)
}

func TestLoadFromDirs_duplicateStockAcrossDirsIsFatal(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(d1, "app.yaml"), []byte(profileYAML("a_*")), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(d2, "app.yaml"), []byte(profileYAML("b_*")), 0o600))

	_, err := LoadFromDirs([]DirSpec{{Path: d1, IsStock: true}, {Path: d2, IsStock: true}})
	assert.Error(t, err)
}

func TestLoadFromDirs_duplicateUserAcrossDirsKeepsFirst(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(d1, "app.yaml"), []byte(profileYAML("first_*")), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(d2, "app.yaml"), []byte(profileYAML("second_*")), 0o600))

	cat, err := LoadFromDirs([]DirSpec{{Path: d1, IsStock: false}, {Path: d2, IsStock: false}})
	require.NoError(t, err)
	require.Len(t, cat.OrderedProfiles(), 1)
	assert.Equal(t, "first_*", cat.OrderedProfiles()[0].Match)
}

func TestLoadFromDirs_missingStockDirIsFatal(t *testing.T) {
	_, err := LoadFromDirs([]DirSpec{{Path: filepath.Join(t.TempDir(), "nope"), IsStock: true}})
	assert.Error(t, err)
}

func TestLoadFromDirs_missingUserDirIsSkipped(t *testing.T) {
	cat, err := LoadFromDirs([]DirSpec{{Path: filepath.Join(t.TempDir(), "nope"), IsStock: false}})
	require.NoError(t, err)
	assert.True(t, cat.Empty())
}

func TestLoadFromDirs_emptySpecs(t *testing.T) {
	cat, err := LoadFromDirs(nil)
	require.NoError(t, err)
	assert.True(t, cat.Empty())
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
