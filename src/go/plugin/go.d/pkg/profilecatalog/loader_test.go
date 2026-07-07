// SPDX-License-Identifier: GPL-3.0-or-later

package profilecatalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tProfile is a minimal profile type for exercising the loader mechanics.
type tProfile struct {
	Name    string
	Content string
}

// decodeTest treats the file content as the profile body. Content "BAD" fails
// decoding, so decode-error handling (stock fatal / user skip) can be tested.
func decodeTest(ctx FileContext, data []byte) (tProfile, error) {
	s := strings.TrimSpace(string(data))
	if s == "BAD" {
		return tProfile{}, errors.New("bad profile")
	}
	return tProfile{Name: ctx.BaseName, Content: s}, nil
}

// dirFiles describes one search directory to materialize under a temp root.
type dirFiles struct {
	isStock bool
	files   map[string]string // relative path -> content
}

// buildSpecs writes each dirFiles into its own temp dir and returns the specs in
// order.
func buildSpecs(t *testing.T, dirs []dirFiles) []DirSpec {
	t.Helper()
	specs := make([]DirSpec, 0, len(dirs))
	for _, d := range dirs {
		root := t.TempDir()
		for name, content := range d.files {
			p := filepath.Join(root, name)
			require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
			require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		}
		specs = append(specs, DirSpec{Path: root, IsStock: d.isStock})
	}
	return specs
}

func names(profiles []Named[tProfile]) []string {
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Name)
	}
	return out
}

func TestLoad(t *testing.T) {
	tests := map[string]struct {
		dirs      []dirFiles
		normalize func(string) string
		// useMissingStock inserts a non-existent stock dir spec.
		useMissingStock bool
		// useMissingUser inserts a non-existent user dir spec.
		useMissingUser bool
		wantErr        bool
		wantInOrder    []string // basenames in discovery order; nil skips the check
		wantContent    map[string]string
	}{
		"single stock profile": {
			dirs:        []dirFiles{{isStock: true, files: map[string]string{"app.yaml": "one"}}},
			wantInOrder: []string{"app"},
			wantContent: map[string]string{"app": "one"},
		},
		"ignores non-yaml and underscore-prefixed files": {
			dirs: []dirFiles{{isStock: true, files: map[string]string{
				"app.yaml":      "one",
				"_partial.yaml": "skip",
				"notes.txt":     "skip",
				"readme.md":     "skip",
			}}},
			wantInOrder: []string{"app"},
		},
		"accepts .yml extension": {
			dirs:        []dirFiles{{isStock: true, files: map[string]string{"app.yml": "one"}}},
			wantInOrder: []string{"app"},
		},
		"recurses into subdirectories": {
			dirs:        []dirFiles{{isStock: true, files: map[string]string{"nested/app.yaml": "one"}}},
			wantInOrder: []string{"app"},
		},
		"invalid basename in stock is fatal": {
			dirs:    []dirFiles{{isStock: true, files: map[string]string{"App.yaml": "one"}}},
			wantErr: true,
		},
		"invalid basename in user is skipped": {
			dirs: []dirFiles{
				{isStock: true, files: map[string]string{"good.yaml": "g"}},
				{isStock: false, files: map[string]string{"App.yaml": "bad-name"}},
			},
			wantInOrder: []string{"good"},
		},
		"decode error in stock is fatal": {
			dirs:    []dirFiles{{isStock: true, files: map[string]string{"app.yaml": "BAD"}}},
			wantErr: true,
		},
		"decode error in user is skipped, keeps others": {
			dirs: []dirFiles{
				{isStock: true, files: map[string]string{"good.yaml": "g"}},
				{isStock: false, files: map[string]string{"bad.yaml": "BAD"}},
			},
			wantInOrder: []string{"good"},
		},
		"user overrides stock (stock dir first)": {
			dirs: []dirFiles{
				{isStock: true, files: map[string]string{"app.yaml": "stock"}},
				{isStock: false, files: map[string]string{"app.yaml": "user"}},
			},
			wantInOrder: []string{"app"},
			wantContent: map[string]string{"app": "user"},
		},
		"user overrides stock (user dir first)": {
			dirs: []dirFiles{
				{isStock: false, files: map[string]string{"app.yaml": "user"}},
				{isStock: true, files: map[string]string{"app.yaml": "stock"}},
			},
			wantInOrder: []string{"app"},
			wantContent: map[string]string{"app": "user"},
		},
		"duplicate stock across dirs is fatal": {
			dirs: []dirFiles{
				{isStock: true, files: map[string]string{"app.yaml": "one"}},
				{isStock: true, files: map[string]string{"app.yaml": "two"}},
			},
			wantErr: true,
		},
		"duplicate stock in nested subdir is fatal": {
			dirs: []dirFiles{{isStock: true, files: map[string]string{
				"app.yaml":        "one",
				"nested/app.yaml": "two",
			}}},
			wantErr: true,
		},
		"duplicate stock is fatal even when a user profile shadows it (user first)": {
			dirs: []dirFiles{
				{isStock: false, files: map[string]string{"app.yaml": "user"}},
				{isStock: true, files: map[string]string{"app.yaml": "stockA"}},
				{isStock: true, files: map[string]string{"app.yaml": "stockB"}},
			},
			wantErr: true,
		},
		"duplicate stock is fatal even with a user override between them": {
			dirs: []dirFiles{
				{isStock: true, files: map[string]string{"app.yaml": "stockA"}},
				{isStock: false, files: map[string]string{"app.yaml": "user"}},
				{isStock: true, files: map[string]string{"app.yaml": "stockB"}},
			},
			wantErr: true,
		},
		"duplicate user across dirs keeps first": {
			dirs: []dirFiles{
				{isStock: false, files: map[string]string{"app.yaml": "first"}},
				{isStock: false, files: map[string]string{"app.yaml": "second"}},
			},
			wantInOrder: []string{"app"},
			wantContent: map[string]string{"app": "first"},
		},
		"missing stock dir is fatal": {
			useMissingStock: true,
			wantErr:         true,
		},
		"missing user dir is skipped": {
			dirs:           []dirFiles{{isStock: true, files: map[string]string{"app.yaml": "one"}}},
			useMissingUser: true,
			wantInOrder:    []string{"app"},
		},
		"empty specs yields empty catalog, no error": {
			dirs:        nil,
			wantInOrder: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			specs := buildSpecs(t, tc.dirs)
			if tc.useMissingStock {
				specs = append(specs, DirSpec{Path: filepath.Join(t.TempDir(), "nope"), IsStock: true})
			}
			if tc.useMissingUser {
				specs = append(specs, DirSpec{Path: filepath.Join(t.TempDir(), "nope"), IsStock: false})
			}

			cat, err := Load(specs, Options[tProfile]{Decode: decodeTest, NormalizeKey: tc.normalize})

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tc.wantInOrder != nil {
				assert.Equal(t, tc.wantInOrder, names(cat.InOrder()))
			}

			for base, want := range tc.wantContent {
				got, ok := cat.Get(base)
				require.True(t, ok, "profile %q must exist", base)
				assert.Equal(t, want, got.Content)
			}
		})
	}
}

func TestLoad_requiresDecode(t *testing.T) {
	_, err := Load[tProfile](nil, Options[tProfile]{})
	assert.Error(t, err)
}

func TestLoad_discoveryOrderPreserved(t *testing.T) {
	// filepath.WalkDir visits lexically; InOrder must reflect that.
	specs := buildSpecs(t, []dirFiles{{isStock: true, files: map[string]string{
		"c.yaml": "3", "a.yaml": "1", "b.yaml": "2",
	}}})
	cat, err := Load(specs, Options[tProfile]{Decode: decodeTest})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, names(cat.InOrder()))
}

func TestLoad_caseInsensitiveNormalization(t *testing.T) {
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	specs := buildSpecs(t, []dirFiles{{isStock: true, files: map[string]string{"app.yaml": "one"}}})
	cat, err := Load(specs, Options[tProfile]{Decode: decodeTest, NormalizeKey: norm})
	require.NoError(t, err)

	got, ok := cat.Get("APP")
	require.True(t, ok, "case-insensitive lookup must resolve")
	assert.Equal(t, "app", got.Name)
	assert.True(t, cat.Has("App"))
}

// TestLoad_stockNamesUsesStockBasename verifies that StockNames/HasStock report
// the stock profile's own basename even when a user profile with a differently
// cased basename overrides it (custom case-insensitive normalization).
func TestLoad_stockNamesUsesStockBasename(t *testing.T) {
	norm := func(s string) string { return strings.ToLower(s) }
	anyName := func(string) bool { return true }
	specs := buildSpecs(t, []dirFiles{
		{isStock: true, files: map[string]string{"app.yaml": "stock"}},
		{isStock: false, files: map[string]string{"App.yaml": "user"}},
	})

	cat, err := Load(specs, Options[tProfile]{Decode: decodeTest, NormalizeKey: norm, ValidName: anyName})
	require.NoError(t, err)

	// The user profile ("App") wins the lookup.
	got, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, "user", got.Content)

	// StockNames reports the STOCK basename ("app"), not the winner's ("App").
	assert.Equal(t, []string{"app"}, cat.StockNames())
	assert.True(t, cat.HasStock("APP"))
}

func TestLoad_customValidName(t *testing.T) {
	// A ValidName that rejects everything makes a stock profile fatal.
	specs := buildSpecs(t, []dirFiles{{isStock: true, files: map[string]string{"app.yaml": "one"}}})
	_, err := Load(specs, Options[tProfile]{
		Decode:    decodeTest,
		ValidName: func(string) bool { return false },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid basename "app"`)
	assert.NotContains(t, err.Error(), reValidName.String())
}

func TestDefaultValidName(t *testing.T) {
	tests := map[string]bool{
		"app":        true,
		"app_1":      true,
		"haproxy":    true,
		"App":        false,
		"1app":       false,
		"app-1":      false,
		"":           false,
		"with.dot":   false,
		"with space": false,
	}
	for in, want := range tests {
		assert.Equalf(t, want, DefaultValidName(in), "DefaultValidName(%q)", in)
	}
}

func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	assert.True(t, DirExists(dir))
	assert.False(t, DirExists(filepath.Join(dir, "nope")))

	f := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	assert.False(t, DirExists(f), "a regular file is not a directory")
}
