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
		"accepts uppercase YAML extension": {
			dirs:        []dirFiles{{isStock: true, files: map[string]string{"app.YAML": "one"}}},
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

func TestLoad_requiresExactlyOneLoader(t *testing.T) {
	loadFile := func(FileContext) (tProfile, error) { return tProfile{}, nil }

	_, err := Load[tProfile](nil, Options[tProfile]{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")

	_, err = Load(nil, Options[tProfile]{Decode: decodeTest, LoadFile: loadFile})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")
}

func TestLoad_loadFileAndCompoundFileName(t *testing.T) {
	specs := buildSpecs(t, []dirFiles{{isStock: true, files: map[string]string{
		"app.yaml.zst": "compressed",
		"notes.txt":    "ignored",
	}}})
	var parsed []string
	var loaded []FileContext

	cat, err := Load(specs, Options[tProfile]{
		LoadFile: func(ctx FileContext) (tProfile, error) {
			loaded = append(loaded, ctx)
			return tProfile{Name: ctx.BaseName, Content: filepath.Base(ctx.Path)}, nil
		},
		ParseFileName: func(name string) (string, bool) {
			parsed = append(parsed, name)
			const suffix = ".yaml.zst"
			if !strings.HasSuffix(name, suffix) {
				return "", false
			}
			return strings.TrimSuffix(name, suffix), true
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"app.yaml.zst", "notes.txt"}, parsed)
	require.Len(t, loaded, 1)
	assert.Equal(t, "app", loaded[0].BaseName)
	assert.True(t, loaded[0].IsStock)
	assert.Equal(t, "app.yaml.zst", filepath.Base(loaded[0].Path))
	got, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, "app.yaml.zst", got.Content)
}

func TestLoad_compoundIdentityPrecedenceAndDuplicates(t *testing.T) {
	parse := func(name string) (string, bool) {
		for _, suffix := range []string{".yaml.zst", ".yml.zst", ".yaml", ".yml"} {
			if before, ok := strings.CutSuffix(name, suffix); ok {
				return before, true
			}
		}
		return "", false
	}
	loadFile := func(ctx FileContext) (tProfile, error) {
		return tProfile{Name: ctx.BaseName, Content: filepath.Base(ctx.Path)}, nil
	}

	t.Run("raw user overrides compressed stock", func(t *testing.T) {
		specs := buildSpecs(t, []dirFiles{
			{isStock: true, files: map[string]string{"app.yaml.zst": "stock"}},
			{isStock: false, files: map[string]string{"app.yaml": "user"}},
		})
		cat, err := Load(specs, Options[tProfile]{LoadFile: loadFile, ParseFileName: parse, UserErrors: FailInvalidUser})
		require.NoError(t, err)
		got, ok := cat.Get("app")
		require.True(t, ok)
		assert.Equal(t, "app.yaml", got.Content)
		assert.False(t, cat.EffectiveIsStock("app"))
	})

	t.Run("stock cross-encoding duplicate is fatal", func(t *testing.T) {
		specs := buildSpecs(t, []dirFiles{{isStock: true, files: map[string]string{
			"app.yaml":     "raw",
			"app.yaml.zst": "compressed",
		}}})
		_, err := Load(specs, Options[tProfile]{LoadFile: loadFile, ParseFileName: parse})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate stock profile")
	})

	t.Run("strict user cross-encoding duplicate is fatal", func(t *testing.T) {
		specs := buildSpecs(t, []dirFiles{{isStock: false, files: map[string]string{
			"app.yaml":     "raw",
			"app.yaml.zst": "compressed",
		}}})
		_, err := Load(specs, Options[tProfile]{LoadFile: loadFile, ParseFileName: parse, UserErrors: FailInvalidUser})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate user profile")
	})
}

func TestLoad_userErrorPolicy(t *testing.T) {
	loadFile := func(ctx FileContext) (tProfile, error) {
		if !ctx.IsStock && ctx.BaseName == "app" {
			return tProfile{}, errors.New("invalid user override")
		}
		return tProfile{Name: ctx.BaseName, Content: "stock"}, nil
	}

	for _, userFirst := range []bool{false, true} {
		name := "stock first"
		if userFirst {
			name = "user first"
		}
		t.Run(name, func(t *testing.T) {
			stock := dirFiles{isStock: true, files: map[string]string{"app.yaml": "stock"}}
			user := dirFiles{isStock: false, files: map[string]string{"app.yaml": "bad"}}
			dirs := []dirFiles{stock, user}
			if userFirst {
				dirs = []dirFiles{user, stock}
			}
			specs := buildSpecs(t, dirs)

			cat, err := Load(specs, Options[tProfile]{LoadFile: loadFile})
			require.NoError(t, err)
			got, ok := cat.Get("app")
			require.True(t, ok)
			assert.Equal(t, "stock", got.Content)

			_, err = Load(specs, Options[tProfile]{LoadFile: loadFile, UserErrors: FailInvalidUser})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid user override")
		})
	}

	t.Run("strict missing user directory remains optional", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		cat, err := Load([]DirSpec{{Path: missing}}, Options[tProfile]{LoadFile: loadFile, UserErrors: FailInvalidUser})
		require.NoError(t, err)
		assert.True(t, cat.Empty())
	})

	t.Run("strict non-directory user path fails", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "profiles")
		require.NoError(t, os.WriteFile(path, []byte("not a directory"), 0o644))
		_, err := Load([]DirSpec{{Path: path}}, Options[tProfile]{LoadFile: loadFile, UserErrors: FailInvalidUser})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("unknown policy fails before walking", func(t *testing.T) {
		_, err := Load[tProfile](nil, Options[tProfile]{LoadFile: loadFile, UserErrors: UserErrorPolicy(99)})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid user error policy")
	})
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
