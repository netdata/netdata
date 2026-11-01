// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestProcessAutoEnableRejectsInvalidConfigBeforeDependencyWait(t *testing.T) {
	source := &testDependencySource{callbacks: make(map[int]func(string, string))}
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	cfg := testProductionProcessConfig(reader, output)
	cfg.AutoEnable = true
	cfg.Services = []ProcessService{source}
	creator := lighthouse.Creator(lighthouse.Dependencies{})
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
}

// A failed browser probe is an operational error, not malformed input.
func TestProcessStockJourneyRetryRecoversHiddenFailure(t *testing.T) {
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	cfg := testProductionProcessConfig(reader, output)
	cfg.AutoEnable = true
	executor := &admissionTestJourneyExecutor{}
	creator := journey.Creator(journey.Dependencies{
		Executor: executor,
		Registry: syntheticregistry.New(),
	})
	cfg.Modules["test"] = creator
	cfg.DiscoveryProviders = []agentdiscovery.ProviderFactory{
		agentdiscovery.NewProviderFactory(
			"test",
			func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
				return runTestDiscoverer{
					configs: []confgroup.Config{
						{
							"module":              "test",
							"name":                "site",
							"script":              "export {};",
							"autodetection_retry": 1,
							"__source_type__":     confgroup.TypeStock,
							"__source__":          "test",
							"__provider__":        "test",
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	output.waitContains(t, "CONFIG go.d:collector:test:site delete")
	require.Positive(t, executor.checks.Load(), "initial rejection must come from Check")
	// Failed stock is hidden, but its admitted enabled binding owns retry
	// independently of the public graph projection.
	executor.ready.Store(true)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("process output: %s", output.String())
			for _, event := range diagnostics.snapshot() {
				t.Logf("process diagnostic: %+v", event)
			}
		}
	})
	// Real scheduler ticks retry the removed stock job through the kernel.
	require.Eventually(t, func() bool {
		return strings.Contains(output.String(), "CONFIG go.d:collector:test:site status running")
	}, 6*time.Second, 10*time.Millisecond, "stock journey did not recover through automatic retry")
	for _, event := range diagnostics.snapshot() {
		if event.Err != nil {
			require.NotContains(t, event.Err.Error(), "yielded work is unavailable")
		}
	}
}

type admissionTestJourneyExecutor struct {
	ready  atomic.Bool
	checks atomic.Int32
}

func (e *admissionTestJourneyExecutor) Check(context.Context, synthetic.Kind) error {
	e.checks.Add(1)
	if !e.ready.Load() {
		return errors.New("browser probe unavailable")
	}
	return nil
}
func (*admissionTestJourneyExecutor) Execute(
	ctx context.Context, _ synthetic.Request, _ func(string),
) synthetic.Execution {
	<-ctx.Done()
	return synthetic.Execution{Drained: true}
}
