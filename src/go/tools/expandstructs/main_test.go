// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandDoesNotDependOnExternalFormatter(t *testing.T) {
	t.Setenv("PATH", "")
	path := filepath.Join(t.TempDir(), "fixture.go")
	require.NoError(t, os.WriteFile(path, []byte(`package fixture

type item struct {
	name  string
	count int
}

var value = item{name: "example", count: 1}
`), 0o644))

	require.NoError(t, expand(path))
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, `package fixture

type item struct {
	name  string
	count int
}

var value = item{
	name:  "example",
	count: 1,
}
`, string(actual))
}
