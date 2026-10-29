// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package w1sensor

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemperatureErrorsDoNotExposeContent(t *testing.T) {
	for name, test := range map[string]struct {
		value  string
		reason error
	}{
		"syntax": {value: "private_temperature_marker", reason: strconv.ErrSyntax},
		"range":  {value: "999999999999999999999999999999", reason: strconv.ErrRange},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "28-test")
			require.NoError(t, os.Mkdir(dir, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "w1_slave"), []byte("crc YES\nvalue t="+test.value+"\n"), 0600))
			c := New()
			c.SensorsPath = root
			require.NoError(t, c.Init(t.Context()))
			err := c.Check(t.Context())
			require.ErrorIs(t, err, test.reason)
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				assert.NotContains(t, cause.Error(), test.value)
			}
		})
	}
}

func BenchmarkReadTemperature(b *testing.B) {
	filename := filepath.Join(b.TempDir(), "w1_slave")
	require.NoError(b, os.WriteFile(filename, []byte("crc YES\nvalue t=22500\n"), 0600))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := readW1sensorTemperature(filename); err != nil {
			b.Fatal(err)
		}
	}
}
