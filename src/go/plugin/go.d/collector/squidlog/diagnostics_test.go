// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const diagnosticMarker = "SYNTHETIC_SOURCE_MARKER"

func TestAssignmentDiagnostics(t *testing.T) {
	collecttest.TestLogLineAssignmentDiagnostics(t, func() logs.LogLine { return newEmptyLogLine() },
		checkCSVFormatField, "resp_time", diagnosticMarker, errBadRespTime)
}

func TestVerificationDiagnostics(t *testing.T) {
	line := newEmptyLogLine()
	require.NoError(t, line.Assign("server_address", diagnosticMarker+" invalid"))
	err := line.verify()
	require.Error(t, err)
	assert.ErrorIs(t, err, errBadServerAddr)
	collecttest.AssertErrorOmits(t, err, diagnosticMarker)
}

func BenchmarkLogLineDiagnostics(b *testing.B) {
	for name, value := range map[string]string{"valid": "200", "invalid": diagnosticMarker} {
		b.Run(name, func(b *testing.B) {
			line := newEmptyLogLine()
			b.ReportAllocs()
			for b.Loop() {
				_ = line.Assign("resp_time", value)
			}
		})
	}
}
