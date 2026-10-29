// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package litespeed

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

func TestReportDiagnosticsDoNotExposeContent(t *testing.T) {
	const marker = "private_report_marker"
	filename := filepath.Join(t.TempDir(), ".rtreport")
	require.NoError(t, os.WriteFile(filename, []byte("BPS_IN: 1, "+marker+", BPS_OUT: "+marker+"\n"), 0600))
	var output bytes.Buffer
	c := New()
	c.Logger = logger.NewWithWriter(&output)
	logger.Level.Set(slog.LevelDebug)
	t.Cleanup(func() { logger.Level.Set(slog.LevelInfo) })
	mx := make(map[string]int64)
	require.NoError(t, c.collectReport(mx, filename))
	assert.EqualValues(t, 8, mx["bps_in"])
	assert.Contains(t, output.String(), "missing colon")
	assert.Contains(t, output.String(), "invalid value")
	assert.NotContains(t, output.String(), marker)
}

func BenchmarkCollectReport(b *testing.B) {
	filename := filepath.Join(b.TempDir(), ".rtreport")
	require.NoError(b, os.WriteFile(filename, []byte("BPS_IN: 1, BPS_OUT: 2\nPLAINCONN: 3, AVAILCONN: 4\n"), 0600))
	c := New()
	c.Logger.Mute()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := c.collectReport(make(map[string]int64), filename); err != nil {
			b.Fatal(err)
		}
	}
}
