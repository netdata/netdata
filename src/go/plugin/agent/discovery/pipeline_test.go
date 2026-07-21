// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func TestPipelineGenerationConstruction(t *testing.T) {
	build := BuildContext{
		Registry: confgroup.Registry{"module": {}},
	}
	tests := map[string]struct {
		catalog      []ProviderFactory
		wantEnabled  []string
		wantDisabled []string
		wantErr      bool
	}{
		"stable enabled and disabled providers": {
			catalog: []ProviderFactory{
				NewProviderFactory(
					"service-discovery",
					func(BuildContext) (Discoverer, bool, error) {
						return pipelineTestDiscoverer{}, true, nil
					},
				),
				NewProviderFactory(
					"disabled",
					func(BuildContext) (Discoverer, bool, error) {
						return nil, false, nil
					},
				),
				NewProviderFactory(
					"file",
					func(BuildContext) (Discoverer, bool, error) {
						return pipelineTestDiscoverer{}, true, nil
					},
				),
			},
			wantEnabled:  []string{"file", "service-discovery"},
			wantDisabled: []string{"disabled"},
		},
		"all disabled": {
			catalog: []ProviderFactory{
				NewProviderFactory(
					"disabled",
					func(BuildContext) (Discoverer, bool, error) {
						return nil, false, nil
					},
				),
			},
			wantErr: true,
		},
		"enabled nil provider": {
			catalog: []ProviderFactory{
				NewProviderFactory(
					"nil",
					func(BuildContext) (Discoverer, bool, error) {
						return nil, true, nil
					},
				),
			},
			wantErr: true,
		},
		"provider build failure": {
			catalog: []ProviderFactory{
				NewProviderFactory(
					"failed",
					func(BuildContext) (Discoverer, bool, error) {
						return nil, false, errors.New("build failed")
					},
				),
			},
			wantErr: true,
		},
		"provider build panic": {
			catalog: []ProviderFactory{
				NewProviderFactory(
					"panicked",
					func(BuildContext) (Discoverer, bool, error) {
						panic("build panic")
					},
				),
			},
			wantErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			catalog, err := NewProviderCatalog(test.catalog)
			if err != nil {
				t.Fatal(err)
			}
			generation, err := NewPipelineGeneration(PipelineConfig{
				BuildContext: build,
				Providers:    catalog,
			})
			if test.wantErr {
				if err == nil {
					t.Fatalf("generation=%+v, want error", generation)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertPipelineNames(
				t,
				generation.ProviderNames(),
				test.wantEnabled,
			)
			assertPipelineNames(
				t,
				generation.DisabledProviderNames(),
				test.wantDisabled,
			)
		})
	}
}

func TestPipelineGenerationRunsNamedProvidersAndAggregates(t *testing.T) {
	factories := make([]ProviderFactory, 0, 2)
	for _, source := range []string{"file", "service-discovery"} {
		factories = append(factories, NewProviderFactory(
			source,
			func(BuildContext) (Discoverer, bool, error) {
				return pipelineTestDiscoverer{
					groups: []*confgroup.Group{{Source: source}},
				}, true, nil
			},
		))
	}
	catalog, err := NewProviderCatalog(factories)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := NewPipelineGeneration(PipelineConfig{
		BuildContext: BuildContext{
			Registry: confgroup.Registry{"module": {}},
		},
		Providers: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	generation.sendEvery = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan []*confgroup.Group, 2)
	runErr := make(chan error, 1)
	go func() {
		runErr <- generation.Run(
			ctx,
			func(
				_ context.Context,
				groups []*confgroup.Group,
			) error {
				out <- groups
				return nil
			},
		)
	}()
	var providers sync.WaitGroup
	for _, name := range generation.ProviderNames() {
		providers.Go(func() {
			if err := generation.RunProvider(ctx, name); err != nil {
				t.Errorf("provider %q: %v", name, err)
			}
		})
	}
	providers.Wait()

	sources := make(map[string]struct{})
	for len(sources) < 2 {
		select {
		case groups := <-out:
			for _, group := range groups {
				sources[group.Source] = struct{}{}
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out with sources=%v", sources)
		}
	}
	if err := generation.RunProvider(
		ctx,
		generation.ProviderNames()[0],
	); err == nil {
		t.Fatal("provider started twice")
	}
	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pipeline aggregation did not stop")
	}
}

func TestPipelineGenerationPropagatesApplyFailure(t *testing.T) {
	wantErr := errors.New("decision failed")
	catalog, err := NewProviderCatalog([]ProviderFactory{
		NewProviderFactory(
			"provider",
			func(BuildContext) (Discoverer, bool, error) {
				return pipelineTestDiscoverer{
					groups: []*confgroup.Group{{Source: "source"}},
				}, true, nil
			},
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	generation, err := NewPipelineGeneration(PipelineConfig{
		BuildContext: BuildContext{
			Registry: confgroup.Registry{"module": {}},
		},
		Providers: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	runErr := make(chan error, 1)
	go func() {
		runErr <- generation.Run(
			ctx,
			func(context.Context, []*confgroup.Group) error {
				return wantErr
			},
		)
	}()
	if err := generation.RunProvider(ctx, "provider"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runErr:
		if !errors.Is(err, wantErr) {
			t.Fatalf("aggregation error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("aggregation did not report decision failure")
	}
}

func TestPipelineGenerationHasNoFixedProviderPopulationCeiling(
	t *testing.T,
) {
	const population = 300
	factories := make([]ProviderFactory, 0, population)
	for ordinal := range population {
		factories = append(
			factories,
			NewProviderFactory(
				fmt.Sprintf("provider-%03d", ordinal),
				func(BuildContext) (Discoverer, bool, error) {
					return pipelineTestDiscoverer{}, true, nil
				},
			),
		)
	}
	catalog, err := NewProviderCatalog(factories)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := NewPipelineGeneration(PipelineConfig{
		BuildContext: BuildContext{
			Registry: confgroup.Registry{"module": {}},
		},
		Providers: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(generation.ProviderNames()); got != population {
		t.Fatalf("providers=%d, want %d", got, population)
	}
}

func BenchmarkBPipelineGenerationRun(b *testing.B) {
	catalog, err := NewProviderCatalog([]ProviderFactory{
		NewProviderFactory(
			"provider",
			func(BuildContext) (Discoverer, bool, error) {
				return pipelineTestDiscoverer{
					groups: []*confgroup.Group{{Source: "source"}},
				}, true, nil
			},
		),
	})
	if err != nil {
		b.Fatal(err)
	}
	config := PipelineConfig{
		BuildContext: BuildContext{
			Registry: confgroup.Registry{"module": {}},
		},
		Providers: catalog,
	}
	b.ReportAllocs()
	for b.Loop() {
		generation, err := NewPipelineGeneration(config)
		if err != nil {
			b.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		runErr := make(chan error, 1)
		go func() {
			runErr <- generation.Run(
				ctx,
				func(context.Context, []*confgroup.Group) error {
					return nil
				},
			)
		}()
		if err := generation.RunProvider(ctx, "provider"); err != nil {
			cancel()
			b.Fatal(err)
		}
		cancel()
		if err := <-runErr; err != nil {
			b.Fatal(err)
		}
	}
}

func assertPipelineNames(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("names=%v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("names=%v, want %v", got, want)
		}
	}
}

type pipelineTestDiscoverer struct {
	groups []*confgroup.Group
}

func (discoverer pipelineTestDiscoverer) Run(
	ctx context.Context,
	out chan<- []*confgroup.Group,
) {
	select {
	case <-ctx.Done():
	case out <- discoverer.groups:
	}
	close(out)
}
