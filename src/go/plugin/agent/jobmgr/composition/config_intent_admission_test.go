// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

type admissionTestIntentOwner struct{ committed atomic.Int32 }
type admissionTestIntentReservation struct{ owner *admissionTestIntentOwner }

func (o *admissionTestIntentOwner) Prepare(
	previous, next collectorapi.JobConfigIdentity,
	config map[string]any,
) (collectorapi.PreparedJobConfigIntent, error) {
	return &admissionTestIntentReservation{o}, nil
}
func (r *admissionTestIntentReservation) Commit()    { r.owner.committed.Add(1) }
func (*admissionTestIntentReservation) Abort() error { return nil }

func TestProcessAutoEnableInvalidConfigDoesNotCommitIntent(t *testing.T) {
	source := &testDependencySource{callbacks: make(map[int]func(string, string))}
	owner := &admissionTestIntentOwner{}
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	cfg := testProductionProcessConfig(reader, output)
	cfg.AutoEnable = true
	cfg.Services = []ProcessService{source}
	creator := lighthouse.Creator(lighthouse.Dependencies{})
	creator.JobConfigIntent = owner
	creator.ActivationDependencies = func(map[string]any) ([]collectorapi.ActivationDependency, error) {
		return []collectorapi.ActivationDependency{
			{Kind: "dem-runtime", Name: "browser", Ready: func() error { return errors.New("runtime missing") }},
		}, nil
	}
	cfg.Modules["test"] = creator
	cfg.DiscoveryProviders = []agentdiscovery.ProviderFactory{
		agentdiscovery.NewProviderFactory(
			"test",
			func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
				return runTestDiscoverer{
					configs: []confgroup.Config{
						{
							"module":          "test",
							"name":            "site",
							"url":             map[string]any{"bad": "shape"},
							"__source_type__": "user",
							"__source__":      "test",
							"__provider__":    "test",
						},
					},
				}, true, nil
			},
		),
	}
	process, err := NewProcess(cfg)
	require.NoError(t, err)
	diagnostics := &recordingCompositionDiagnosticObserver{}
	process.core.diagnostics = diagnostics
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	t.Cleanup(func() {
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		defer cancel()
		defer reader.Close()
		defer writer.Close()
		require.NoError(t, process.Terminate(shutdown))
		require.NoError(t, <-done)
	})
	require.Eventually(t, func() bool {
		for _, event := range diagnostics.snapshot() {
			if jobmgr.IsProposalRejection(event.Err) {
				return true
			}
		}
		return false
	}, 3*time.Second, time.Millisecond)
	require.NotContains(t, output.String(), "CONFIG go.d:collector:test:site create accepted")
	require.Zero(t, owner.committed.Load(), "invalid lighthouse config must not commit acquisition intent")
}
