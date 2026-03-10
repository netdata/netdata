// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	tests := map[string]struct {
		buildCfg        func(t *testing.T) map[string]any
		wantErrContains string
		assertCfg       func(t *testing.T, cfg map[string]any)
	}{
		"env ref": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_USER", "admin")
				return map[string]any{"username": "${env:TEST_SECRET_USER}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "admin", cfg["username"])
			},
		},
		"file ref": {
			buildCfg: func(t *testing.T) map[string]any {
				path := filepath.Join(t.TempDir(), "secret.txt")
				require.NoError(t, os.WriteFile(path, []byte("  s3cret\n"), 0600))
				return map[string]any{"password": "${file:" + path + "}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "s3cret", cfg["password"])
			},
		},
		"nested maps": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_HOST", "db.local")
				return map[string]any{
					"database": map[string]any{
						"host": "${env:TEST_SECRET_HOST}",
						"port": 5432,
					},
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				inner := cfg["database"].(map[string]any)
				assert.Equal(t, "db.local", inner["host"])
				assert.Equal(t, 5432, inner["port"])
			},
		},
		"map any any": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_LABEL", "prod")
				return map[string]any{
					"labels": map[any]any{"env": "${env:TEST_SECRET_LABEL}"},
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				labels := cfg["labels"].(map[any]any)
				assert.Equal(t, "prod", labels["env"])
			},
		},
		"array with strings": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_ITEM", "resolved")
				return map[string]any{
					"items": []any{"plain", "${env:TEST_SECRET_ITEM}", 42},
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				items := cfg["items"].([]any)
				assert.Equal(t, "plain", items[0])
				assert.Equal(t, "resolved", items[1])
				assert.Equal(t, 42, items[2])
			},
		},
		"multiple refs in one string": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_DB_USER", "root")
				t.Setenv("TEST_SECRET_DB_PASS", "p@ss")
				return map[string]any{
					"dsn": "${env:TEST_SECRET_DB_USER}:${env:TEST_SECRET_DB_PASS}@tcp(localhost)/db",
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "root:p@ss@tcp(localhost)/db", cfg["dsn"])
			},
		},
		"uppercase shorthand": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("MY_TOKEN", "tok123")
				return map[string]any{"token": "${MY_TOKEN}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "tok123", cfg["token"])
			},
		},
		"lowercase shorthand left alone": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"template": "${lower_case}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "${lower_case}", cfg["template"])
			},
		},
		"unknown scheme error": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${nosuchprovider:secret/data/pass}"}
			},
			wantErrContains: "unknown secret provider 'nosuchprovider'",
		},
		"missing env var error": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"password": "${env:DEFINITELY_NOT_SET_12345}"}
			},
			wantErrContains: "environment variable 'DEFINITELY_NOT_SET_12345' is not set",
		},
		"missing file error": {
			buildCfg: func(t *testing.T) map[string]any {
				path := filepath.Join(t.TempDir(), "nonexistent_secret_file")
				return map[string]any{"secret": "${file:" + path + "}"}
			},
			wantErrContains: "resolving secret '${file:",
		},
		"internal keys skipped": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{
					"__source__":      "${env:SHOULD_NOT_RESOLVE}",
					"__source_type__": "${env:SHOULD_NOT_RESOLVE}",
					"__provider__":    "${env:SHOULD_NOT_RESOLVE}",
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "${env:SHOULD_NOT_RESOLVE}", cfg["__source__"])
				assert.Equal(t, "${env:SHOULD_NOT_RESOLVE}", cfg["__source_type__"])
				assert.Equal(t, "${env:SHOULD_NOT_RESOLVE}", cfg["__provider__"])
			},
		},
		"non-string values untouched": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{
					"port":    8080,
					"enabled": true,
					"ratio":   3.14,
					"nothing": nil,
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, 8080, cfg["port"])
				assert.Equal(t, true, cfg["enabled"])
				assert.Equal(t, 3.14, cfg["ratio"])
				assert.Nil(t, cfg["nothing"])
			},
		},
		"no refs no changes": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"host": "localhost", "port": 3306}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "localhost", cfg["host"])
				assert.Equal(t, 3306, cfg["port"])
			},
		},
		"empty map": {
			buildCfg: func(t *testing.T) map[string]any { return map[string]any{} },
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Empty(t, cfg)
			},
		},
		"missing env var shorthand": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"token": "${MISSING_SHORTHAND_VAR_12345}"}
			},
			wantErrContains: "environment variable 'MISSING_SHORTHAND_VAR_12345' is not set",
		},
		"mixed refs and plain text": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_PROTO", "https")
				return map[string]any{"url": "${env:TEST_SECRET_PROTO}://example.com/api"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "https://example.com/api", cfg["url"])
			},
		},
		"deeply nested": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_DEEP", "found")
				return map[string]any{
					"level1": map[string]any{
						"level2": map[string]any{"level3": "${env:TEST_SECRET_DEEP}"},
					},
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				l1 := cfg["level1"].(map[string]any)
				l2 := l1["level2"].(map[string]any)
				assert.Equal(t, "found", l2["level3"])
			},
		},
		"file ref and env ref together": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_FUSER", "admin")
				path := filepath.Join(t.TempDir(), "pass.txt")
				require.NoError(t, os.WriteFile(path, []byte("hunter2\n"), 0600))
				return map[string]any{
					"dsn": "${env:TEST_SECRET_FUSER}:${file:" + path + "}@host",
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "admin:hunter2@host", cfg["dsn"])
			},
		},
		"internal key in nested map": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{
					"sub": map[string]any{
						"__meta__": "${env:SHOULD_NOT_RESOLVE}",
						"normal":   "plain",
					},
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				sub := cfg["sub"].(map[string]any)
				assert.Equal(t, "${env:SHOULD_NOT_RESOLVE}", sub["__meta__"])
				assert.Equal(t, "plain", sub["normal"])
			},
		},
		"empty env name error": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${env:}"}
			},
			wantErrContains: "environment variable '' is not set",
		},
		"empty file name error": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${file:}"}
			},
			wantErrContains: "resolving secret",
		},
		"multiple refs one failure": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_OK", "good")
				return map[string]any{
					"dsn": "${env:TEST_SECRET_OK}:${env:MISSING_VAR_12345}@host",
				}
			},
			wantErrContains: "MISSING_VAR_12345",
		},
		"array in nested map": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_ARR", "val")
				return map[string]any{
					"outer": map[string]any{
						"list": []any{"${env:TEST_SECRET_ARR}", "static"},
					},
				}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				outer := cfg["outer"].(map[string]any)
				list := outer["list"].([]any)
				assert.Equal(t, "val", list[0])
				assert.Equal(t, "static", list[1])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := tc.buildCfg(t)
			err := Resolve(cfg)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			if tc.assertCfg != nil {
				tc.assertCfg(t, cfg)
			}
		})
	}
}
