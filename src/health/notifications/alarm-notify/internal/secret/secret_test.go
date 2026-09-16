// SPDX-License-Identifier: GPL-3.0-or-later

package secret

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
			got, err := Resolve(context.Background(), test.value)
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

func TestIsReference(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "not-created")
	for name, test := range map[string]struct {
		value     string
		reference bool
		invalid   bool
	}{
		"literal":            {value: "  literal value  "},
		"empty literal":      {},
		"unset environment":  {value: "${env:NOTIFIER_TEST_UNSET_REFERENCE}", reference: true},
		"missing file":       {value: "${file:" + missingFile + "}", reference: true},
		"empty operand":      {value: "${env:}", invalid: true},
		"relative file":      {value: "${file:relative}", invalid: true},
		"unsupported scheme": {value: "${cmd:synthetic-value}", invalid: true},
		"interpolation":      {value: "prefix-${env:NAME}", invalid: true},
		"multiple":           {value: "${env:A}${env:B}", invalid: true},
		"unclosed":           {value: "${env:A", invalid: true},
		"nested":             {value: "${env:${env:NAME}}", invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := IsReference(test.value)
			assert.Equal(t, test.reference, got)
			if test.invalid {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "synthetic-value")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolveCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for name, test := range map[string]struct {
		value string
		want  string
		err   error
	}{
		"literal is unchanged":    {value: "  literal  ", want: "  literal  "},
		"environment is not read": {value: "${env:NOTIFIER_TEST_UNSET_REFERENCE}", err: context.Canceled},
		"file is not read":        {value: "${file:" + filepath.Join(t.TempDir(), "not-created") + "}", err: context.Canceled},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Resolve(ctx, test.value)
			assert.Equal(t, test.want, got)
			assert.ErrorIs(t, err, test.err)
		})
	}
}
