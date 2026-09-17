// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func LoadFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	var document map[string]any
	LoadFixtureInto(t, name, &document)
	return document
}

func LoadFixtureInto(t *testing.T, name string, target any) {
	t.Helper()
	raw, err := os.ReadFile(name)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, target))
}
