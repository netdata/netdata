// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dnsmasq_dhcp

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationDiagnosticsDoNotExposeRawDirectives(t *testing.T) {
	const marker = "private_directive_marker"
	filename := filepath.Join(t.TempDir(), "dnsmasq.conf")
	content := "dhcp-range=set:" + marker + ",192.0.2.10,192.0.2.20,12h\ndhcp-host=" + marker + ",192.0.2.30\n"
	require.NoError(t, os.WriteFile(filename, []byte(content), 0600))
	var output bytes.Buffer
	c := New()
	c.Logger = logger.NewWithWriter(&output)
	c.ConfPath = filename
	c.ConfDir = ""
	logger.Level.Set(slog.LevelDebug)
	t.Cleanup(func() { logger.Level.Set(slog.LevelInfo) })
	ranges, hosts := c.parseDnsmasqDHCPConfiguration()
	require.Len(t, ranges, 1)
	require.Len(t, hosts, 1)
	assert.Equal(t, "192.0.2.30", hosts[0].String())
	assert.Contains(t, output.String(), "192.0.2.10-192.0.2.20")
	assert.Contains(t, output.String(), "192.0.2.30")
	assert.NotContains(t, output.String(), marker)
}

func BenchmarkParseDHCPConfiguration(b *testing.B) {
	c := New()
	c.ConfPath = testConfPath
	c.ConfDir = testConfDir
	c.Logger.Mute()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.parseDnsmasqDHCPConfiguration()
	}
}
