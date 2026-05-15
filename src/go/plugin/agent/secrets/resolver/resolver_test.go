// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
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
		"env ref trims surrounding whitespace": {
			buildCfg: func(t *testing.T) map[string]any {
				t.Setenv("TEST_SECRET_TRIMMED", "  admin \n")
				return map[string]any{"username": "${env:TEST_SECRET_TRIMMED}"}
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
		"uppercase no-scheme left alone": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"token": "${MY_TOKEN}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "${MY_TOKEN}", cfg["token"])
			},
		},
		"lowercase no-scheme left alone": {
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
		"legacy remote vault syntax is rejected": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${vault:secret/data/pass#key}"}
			},
			wantErrContains: "unknown secret provider 'vault'",
		},
		"legacy remote aws syntax is rejected": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${aws-sm:mysecret}"}
			},
			wantErrContains: "unknown secret provider 'aws-sm'",
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
		"relative file path error": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"secret": "${file:relative/secret.txt}"}
			},
			wantErrContains: "file path must be absolute",
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
		"missing uppercase no-scheme left alone": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"token": "${MISSING_SHORTHAND_VAR_12345}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "${MISSING_SHORTHAND_VAR_12345}", cfg["token"])
			},
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
			resolver := New()
			cfg := tc.buildCfg(t)
			err := resolver.Resolve(cfg)

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

func TestResolveWithStoreResolver(t *testing.T) {
	tests := map[string]struct {
		cfg             map[string]any
		storeResolver   StoreRefResolver
		wantErrContains string
		assertCfg       func(t *testing.T, cfg map[string]any)
	}{
		"store ref with resolver": {
			cfg: map[string]any{
				"password": "${store:vault:vault_prod:secret/data/mysql#password}",
			},
			storeResolver: func(ctx context.Context, ref, original string) (string, error) {
				require.NotNil(t, ctx)
				if ref == "vault:vault_prod:secret/data/mysql#password" && original == "${store:vault:vault_prod:secret/data/mysql#password}" {
					return "resolved-secret", nil
				}
				return "", errors.New("unexpected ref")
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "resolved-secret", cfg["password"])
			},
		},
		"store ref without resolver": {
			cfg: map[string]any{
				"password": "${store:vault:vault_prod:secret/data/mysql#password}",
			},
			wantErrContains: "secretstore resolver is not configured",
		},
		"store resolver error bubbles": {
			cfg: map[string]any{
				"password": "${store:vault:vault_prod:secret/data/mysql#password}",
			},
			storeResolver: func(ctx context.Context, ref, original string) (string, error) {
				require.NotNil(t, ctx)
				return "", errors.New("store not configured")
			},
			wantErrContains: "store not configured",
		},
		"mixed env and store refs": {
			cfg: map[string]any{
				"dsn": "${env:TEST_SR_USER}:${store:aws-sm:aws_prod:app/db#password}@host",
			},
			storeResolver: func(ctx context.Context, ref, original string) (string, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, "aws-sm:aws_prod:app/db#password", ref)
				return "p@ss", nil
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "admin:p@ss@host", cfg["dsn"])
			},
		},
		"store resolver receives canceled context": {
			cfg: map[string]any{
				"password": "${store:vault:vault_prod:secret/data/mysql#password}",
			},
			storeResolver: func(ctx context.Context, ref, original string) (string, error) {
				<-ctx.Done()
				return "", ctx.Err()
			},
			wantErrContains: context.Canceled.Error(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if name == "mixed env and store refs" {
				t.Setenv("TEST_SR_USER", "admin")
			}

			resolver := New()
			ctx := context.Background()
			if name == "store resolver receives canceled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}

			err := resolver.ResolveWithStoreResolver(ctx, tc.cfg, tc.storeResolver)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			if tc.assertCfg != nil {
				tc.assertCfg(t, tc.cfg)
			}
		})
	}
}

func TestResolveRefUsesProviderRegistry(t *testing.T) {
	resolver := New()

	called := false
	resolver.providers["stub"] = func(ctx context.Context, ref, original string) (string, error) {
		called = true
		require.NotNil(t, ctx)
		assert.Equal(t, "name", ref)
		assert.Equal(t, "${stub:name}", original)
		return "resolved-by-stub", nil
	}

	cfg := map[string]any{
		"value": "${stub:name}",
	}

	require.NoError(t, resolver.Resolve(cfg))
	assert.True(t, called)
	assert.Equal(t, "resolved-by-stub", cfg["value"])
}

func TestResolveWithStoreResolver_LogsDetailedBuiltinResolution(t *testing.T) {
	modeFile := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(modeFile, []byte("from-file\n"), 0o600))

	tests := map[string]struct {
		cfg           map[string]any
		onWindowsSkip bool
		setup         func(t *testing.T)
		wantLog       string
		dontWantLogs  []string
	}{
		"env": {
			cfg: map[string]any{"value": "${env:TEST_SECRET_ENV}"},
			setup: func(t *testing.T) {
				t.Setenv("TEST_SECRET_ENV", "from-env")
			},
			wantLog:      "resolved secret via env variable 'TEST_SECRET_ENV'",
			dontWantLogs: []string{"from-env"},
		},
		"file": {
			cfg:          map[string]any{"value": "${file:" + modeFile + "}"},
			wantLog:      "resolved secret via file '" + modeFile + "'",
			dontWantLogs: []string{"from-file"},
		},
		"cmd": {
			cfg:           map[string]any{"value": "${cmd:/bin/echo from-cmd}"},
			onWindowsSkip: true,
			wantLog:       "resolved secret via command '/bin/echo'",
			dontWantLogs:  []string{"from-cmd"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.onWindowsSkip && runtime.GOOS == "windows" {
				t.Skip("skipping on windows")
			}
			if tc.setup != nil {
				tc.setup(t)
			}

			out := captureResolverLoggerOutput(t, func(log *logger.Logger) {
				ctx := logger.ContextWithLogger(context.Background(), log)
				require.NoError(t, New().ResolveWithStoreResolver(ctx, tc.cfg, nil))
			})

			assert.Contains(t, out, tc.wantLog)
			for _, s := range tc.dontWantLogs {
				assert.NotContains(t, out, s)
			}
		})
	}
}

func captureResolverLoggerOutput(t *testing.T, fn func(log *logger.Logger)) string {
	t.Helper()

	var buf bytes.Buffer
	fn(logger.NewWithWriter(&buf))
	return buf.String()
}
