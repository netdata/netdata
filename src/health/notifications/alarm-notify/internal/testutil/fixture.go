// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ReadStatusFixture loads a provider-owned payload fixture for a status and event variant.
func ReadStatusFixture(t *testing.T, directory, provider, status, variant string) string {
	t.Helper()
	name := fmt.Sprintf("%s-%s-%s.json", provider, strings.ToLower(status), variant)
	data, err := os.ReadFile(filepath.Join(directory, name))
	require.NoError(t, err)
	return string(data)
}
