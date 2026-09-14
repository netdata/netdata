// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSecret(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secret with spaces")
	require.NoError(t, os.WriteFile(file, []byte("  synthetic-private-value\n"), 0600))
	t.Setenv("NOTIFIER_TEST_SECRET", "  synthetic-private-value\n")
	t.Setenv("NOTIFIER_TEST_EMPTY", "")
	tests := map[string]struct {
		value string
		want  string
		err   string
	}{
		"literal":         {value: "synthetic-private-value", want: "synthetic-private-value"},
		"env":             {value: "${env:NOTIFIER_TEST_SECRET}", want: "synthetic-private-value"},
		"file":            {value: "${file:" + file + "}", want: "synthetic-private-value"},
		"empty env":       {value: "${env:NOTIFIER_TEST_EMPTY}", err: "empty value"},
		"missing file":    {value: "${file:" + file + "-missing}", err: "could not read secret file"},
		"command refused": {value: "${cmd:/synthetic-private-value}", err: "only whole env and file"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveSecret(context.Background(), test.value)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.want, got)
			}
		})
	}
}
