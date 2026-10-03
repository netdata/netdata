// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package isc_dhcpd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDHCPdLeasesFile_MalformedLines(t *testing.T) {
	// dhcpd.leases is rewritten/appended by the dhcpd daemon, so a line can be
	// truncated mid-write at EOF (daemon killed, disk full, process crash).
	// A line like "lease 1" or "iaaddr 2" still matches the keyword prefix
	// but is too short to hold the expected value: it must be skipped instead
	// of panicking on an out-of-range slice.
	tests := map[string]string{
		"'lease' line truncated mid-write":          "lease 1\n",
		"'iaaddr' line truncated mid-write":         "iaaddr 2\n",
		"'binding state' line truncated mid-write":  "lease 192.168.0.1 {\nbinding state\n",
		"'lease' line truncated right after prefix": "lease {\n",
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "dhcpd.leases")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

			var leases []leaseEntry
			var err error
			assert.NotPanics(t, func() {
				leases, err = parseDHCPdLeasesFile(path)
			})
			assert.NoError(t, err)
			assert.Empty(t, leases)
		})
	}
}

func TestParseDHCPdLeasesFile_WellFormed(t *testing.T) {
	content := "" +
		"lease 192.168.0.1 {\n" +
		"  binding state active;\n" +
		"}\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "dhcpd.leases")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	leases, err := parseDHCPdLeasesFile(path)
	require.NoError(t, err)
	require.Len(t, leases, 1)
	assert.Equal(t, "192.168.0.1", leases[0].addr.String())
	assert.Equal(t, "active", leases[0].bindingState)
}
