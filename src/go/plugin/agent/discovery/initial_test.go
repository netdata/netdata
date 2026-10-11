// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type initialTestDiscoverer struct {
	source  string
	release <-chan struct{}
}

func (d initialTestDiscoverer) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	d.RunInitial(ctx, out, func() {})
}
func (d initialTestDiscoverer) RunInitial(ctx context.Context, out chan<- []*confgroup.Group, initial func()) {
	defer close(out)
	if d.release != nil {
		select {
		case <-ctx.Done():
			return
		case <-d.release:
		}
	}
	select {
	case <-ctx.Done():
		return
	case out <- []*confgroup.Group{{Source: d.source}}:
	}
	initial()
}
func initialTestPipeline(t *testing.T, initial func() error, providers ...Discoverer) *PipelineGeneration {
	t.Helper()
	var factories []ProviderFactory
	for i, provider := range providers {
		factories = append(factories, NewProviderFactory(string(rune('a'+i)), func(BuildContext) (Discoverer, bool, error) { return provider, true, nil }))
	}
	catalog, err := NewProviderCatalog(factories)
	require.NoError(t, err)
	pipeline, err := NewPipelineGeneration(PipelineConfig{BuildContext: BuildContext{Registry: confgroup.Registry{"module": {}}}, Providers: catalog, InitialApplied: initial})
	require.NoError(t, err)
	pipeline.sendEvery = time.Hour // Completion must flush pending updates without waiting for the batching tick.
	return pipeline
}
func TestInitialAppliedWaitsForEverySnapshotAndCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	second, initial, firstApply, releaseApply := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var seen []string
	pipeline := initialTestPipeline(t, func() error { close(initial); return nil }, initialTestDiscoverer{source: "first"}, initialTestDiscoverer{source: "second", release: second})
	runDone := make(chan error, 1)
	go func() {
		runDone <- pipeline.Run(ctx, func(_ context.Context, groups []*confgroup.Group) error {
			for _, group := range groups {
				seen = append(seen, group.Source)
			}
			if len(seen) == 1 {
				close(firstApply)
				select {
				case <-releaseApply:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}()
	providerDone := make(chan error, 2)
	for _, name := range pipeline.ProviderNames() {
		go func() { providerDone <- pipeline.RunProvider(ctx, name) }()
	}
	select {
	case <-firstApply:
	case <-time.After(5 * time.Second):
		t.Fatal("initial apply did not start")
	}
	select {
	case <-initial:
		t.Fatal("completed before first commit")
	default:
	}
	close(releaseApply)
	select {
	case <-initial:
		t.Fatal("completed before second snapshot")
	default:
	}
	close(second)
	select {
	case <-initial:
	case <-time.After(5 * time.Second):
		t.Fatal("initial completion did not flush pending snapshot")
	}
	require.Equal(t, []string{"first", "second"}, seen)
	for range 2 {
		require.NoError(t, <-providerDone)
	}
	cancel()
	require.NoError(t, <-runDone)
}
func TestInitialAppliedDoesNotAcknowledgeFailedApply(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	called := false
	pipeline := initialTestPipeline(t, func() error { called = true; return nil }, initialTestDiscoverer{source: "first"})
	done := make(chan error, 1)
	go func() { done <- pipeline.RunProvider(ctx, "a") }()
	failure := errors.New("registration could not commit")
	require.ErrorIs(t, pipeline.Run(ctx, func(context.Context, []*confgroup.Group) error { return failure }), failure)
	require.NoError(t, <-done)
	require.False(t, called)
}
func TestInitialAppliedFailurePropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	failure := errors.New("completion frame could not commit")
	pipeline := initialTestPipeline(t, func() error { return failure }, initialTestDiscoverer{source: "first"})
	done := make(chan error, 1)
	go func() { done <- pipeline.RunProvider(ctx, "a") }()
	require.ErrorIs(t, pipeline.Run(ctx, func(context.Context, []*confgroup.Group) error { return nil }), failure)
	require.NoError(t, <-done)
}
func TestInitialAppliedRequiresExplicitFiniteProviders(t *testing.T) {
	catalog, err := NewProviderCatalog([]ProviderFactory{NewProviderFactory("continuous", func(BuildContext) (Discoverer, bool, error) { return pipelineTestDiscoverer{}, true, nil })})
	require.NoError(t, err)
	_, err = NewPipelineGeneration(PipelineConfig{BuildContext: BuildContext{Registry: confgroup.Registry{"module": {}}}, Providers: catalog, InitialApplied: func() error { return nil }})
	require.ErrorContains(t, err, "no finite initial snapshot")
}
