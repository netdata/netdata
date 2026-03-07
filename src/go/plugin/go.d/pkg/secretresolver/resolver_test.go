// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_EnvRef(t *testing.T) {
	t.Setenv("TEST_SECRET_USER", "admin")

	cfg := map[string]any{
		"username": "${env:TEST_SECRET_USER}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "admin", cfg["username"])
}

func TestResolve_FileRef(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("  s3cret\n"), 0600))

	cfg := map[string]any{
		"password": "${file:" + path + "}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "s3cret", cfg["password"])
}

func TestResolve_NestedMaps(t *testing.T) {
	t.Setenv("TEST_SECRET_HOST", "db.local")

	cfg := map[string]any{
		"database": map[string]any{
			"host": "${env:TEST_SECRET_HOST}",
			"port": 5432,
		},
	}

	require.NoError(t, Resolve(cfg))
	inner := cfg["database"].(map[string]any)
	assert.Equal(t, "db.local", inner["host"])
	assert.Equal(t, 5432, inner["port"])
}

func TestResolve_MapAnyAny(t *testing.T) {
	t.Setenv("TEST_SECRET_LABEL", "prod")

	cfg := map[string]any{
		"labels": map[any]any{
			"env": "${env:TEST_SECRET_LABEL}",
		},
	}

	require.NoError(t, Resolve(cfg))
	labels := cfg["labels"].(map[any]any)
	assert.Equal(t, "prod", labels["env"])
}

func TestResolve_ArrayWithStrings(t *testing.T) {
	t.Setenv("TEST_SECRET_ITEM", "resolved")

	cfg := map[string]any{
		"items": []any{
			"plain",
			"${env:TEST_SECRET_ITEM}",
			42,
		},
	}

	require.NoError(t, Resolve(cfg))
	items := cfg["items"].([]any)
	assert.Equal(t, "plain", items[0])
	assert.Equal(t, "resolved", items[1])
	assert.Equal(t, 42, items[2])
}

func TestResolve_MultipleRefsInOneString(t *testing.T) {
	t.Setenv("TEST_SECRET_DB_USER", "root")
	t.Setenv("TEST_SECRET_DB_PASS", "p@ss")

	cfg := map[string]any{
		"dsn": "${env:TEST_SECRET_DB_USER}:${env:TEST_SECRET_DB_PASS}@tcp(localhost)/db",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "root:p@ss@tcp(localhost)/db", cfg["dsn"])
}

func TestResolve_UppercaseShorthand(t *testing.T) {
	t.Setenv("MY_TOKEN", "tok123")

	cfg := map[string]any{
		"token": "${MY_TOKEN}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "tok123", cfg["token"])
}

func TestResolve_LowercaseLeftAlone(t *testing.T) {
	cfg := map[string]any{
		"template": "${lower_case}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "${lower_case}", cfg["template"])
}

func TestResolve_UnknownSchemeError(t *testing.T) {
	cfg := map[string]any{
		"val": "${vault:secret/data/pass}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown secret provider 'vault'")
}

func TestResolve_MissingEnvVarError(t *testing.T) {
	cfg := map[string]any{
		"password": "${env:DEFINITELY_NOT_SET_12345}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "environment variable 'DEFINITELY_NOT_SET_12345' is not set")
}

func TestResolve_MissingFileError(t *testing.T) {
	cfg := map[string]any{
		"secret": "${file:/tmp/nonexistent_secret_file_12345}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving secret '${file:/tmp/nonexistent_secret_file_12345}'")
}

func TestResolve_InternalKeysSkipped(t *testing.T) {
	cfg := map[string]any{
		"__source__":      "${env:SHOULD_NOT_RESOLVE}",
		"__source_type__": "${env:SHOULD_NOT_RESOLVE}",
		"__provider__":    "${env:SHOULD_NOT_RESOLVE}",
	}

	require.NoError(t, Resolve(cfg))
	// values must remain untouched
	assert.Equal(t, "${env:SHOULD_NOT_RESOLVE}", cfg["__source__"])
	assert.Equal(t, "${env:SHOULD_NOT_RESOLVE}", cfg["__source_type__"])
	assert.Equal(t, "${env:SHOULD_NOT_RESOLVE}", cfg["__provider__"])
}

func TestResolve_NonStringValuesUntouched(t *testing.T) {
	cfg := map[string]any{
		"port":    8080,
		"enabled": true,
		"ratio":   3.14,
		"nothing": nil,
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, 8080, cfg["port"])
	assert.Equal(t, true, cfg["enabled"])
	assert.Equal(t, 3.14, cfg["ratio"])
	assert.Nil(t, cfg["nothing"])
}

func TestResolve_NoRefsNoChanges(t *testing.T) {
	cfg := map[string]any{
		"host": "localhost",
		"port": 3306,
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "localhost", cfg["host"])
	assert.Equal(t, 3306, cfg["port"])
}

func TestResolve_EmptyMap(t *testing.T) {
	cfg := map[string]any{}
	require.NoError(t, Resolve(cfg))
	assert.Empty(t, cfg)
}

func TestResolve_MissingEnvVarShorthand(t *testing.T) {
	cfg := map[string]any{
		"token": "${MISSING_SHORTHAND_VAR_12345}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "environment variable 'MISSING_SHORTHAND_VAR_12345' is not set")
}

func TestResolve_MixedRefsAndPlainText(t *testing.T) {
	t.Setenv("TEST_SECRET_PROTO", "https")

	cfg := map[string]any{
		"url": "${env:TEST_SECRET_PROTO}://example.com/api",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "https://example.com/api", cfg["url"])
}

func TestResolve_DeeplyNested(t *testing.T) {
	t.Setenv("TEST_SECRET_DEEP", "found")

	cfg := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": "${env:TEST_SECRET_DEEP}",
			},
		},
	}

	require.NoError(t, Resolve(cfg))
	l1 := cfg["level1"].(map[string]any)
	l2 := l1["level2"].(map[string]any)
	assert.Equal(t, "found", l2["level3"])
}

func TestResolve_FileRefAndEnvRefTogether(t *testing.T) {
	t.Setenv("TEST_SECRET_FUSER", "admin")
	path := filepath.Join(t.TempDir(), "pass.txt")
	require.NoError(t, os.WriteFile(path, []byte("hunter2\n"), 0600))

	cfg := map[string]any{
		"dsn": "${env:TEST_SECRET_FUSER}:${file:" + path + "}@host",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "admin:hunter2@host", cfg["dsn"])
}

func TestResolve_InternalKeyInNestedMap(t *testing.T) {
	cfg := map[string]any{
		"sub": map[string]any{
			"__meta__": "${env:SHOULD_NOT_RESOLVE}",
			"normal":   "plain",
		},
	}

	require.NoError(t, Resolve(cfg))
	sub := cfg["sub"].(map[string]any)
	assert.Equal(t, "${env:SHOULD_NOT_RESOLVE}", sub["__meta__"])
	assert.Equal(t, "plain", sub["normal"])
}

func TestResolve_ArrayInNestedMap(t *testing.T) {
	t.Setenv("TEST_SECRET_ARR", "val")

	cfg := map[string]any{
		"outer": map[string]any{
			"list": []any{
				"${env:TEST_SECRET_ARR}",
				"static",
			},
		},
	}

	require.NoError(t, Resolve(cfg))
	outer := cfg["outer"].(map[string]any)
	list := outer["list"].([]any)
	assert.Equal(t, "val", list[0])
	assert.Equal(t, "static", list[1])
}
