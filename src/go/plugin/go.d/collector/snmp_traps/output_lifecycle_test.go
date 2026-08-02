// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingOutputStarter struct {
	name  string
	order *[]string
	err   error
}

func (s recordingOutputStarter) Start() error {
	*s.order = append(*s.order, s.name)
	return s.err
}

func TestStartOutputBackendsStartsOTLPBeforeJournal(t *testing.T) {
	var order []string
	require.NoError(t, startOutputBackends(
		recordingOutputStarter{name: "otlp", order: &order},
		recordingOutputStarter{name: "journal", order: &order},
	))
	assert.Equal(t, []string{"otlp", "journal"}, order)
}

func TestStartOutputBackendsStopsAfterFailure(t *testing.T) {
	var order []string
	wantErr := errors.New("start failed")
	err := startOutputBackends(
		recordingOutputStarter{name: "otlp", order: &order, err: wantErr},
		recordingOutputStarter{name: "journal", order: &order},
	)
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, []string{"otlp"}, order)
}

func TestHandleOutputOutcomePreservesBackendAuthorityMetrics(t *testing.T) {
	collector := &Collector{Config: Config{Name: "local"}}
	metrics := &perJobMetrics{}

	collector.handleOutputOutcome(metrics, output.Outcome{
		Backend: output.BackendOTLP, Stage: output.StageEnqueue, FailedEntries: 2,
	})
	assert.Equal(t, uint64(2), metrics.errors.otlpExportFailed.Load())
	assert.Zero(t, metrics.pipeline.writeFailed.Load())

	collector.handleOutputOutcome(metrics, output.Outcome{
		Backend: output.BackendOTLP, Stage: output.StageExport, FailedEntries: 3, Authoritative: true,
	})
	assert.Equal(t, uint64(5), metrics.errors.otlpExportFailed.Load())
	assert.Equal(t, uint64(3), metrics.pipeline.writeFailed.Load())
}
