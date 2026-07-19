package joboutput

import (
	"context"
	"testing"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func BenchmarkBConfigFactoryCold(b *testing.B) {
	state := &factoryTestState{}
	resolver, err := secretresolver.NewAtomicResolver(nil)
	if err != nil {
		b.Fatal(err)
	}
	factory, err := NewConfigModuleFactory(
		ConfigModuleFactoryConfig{
			Modules: collectorapi.Registry{
				"module": {
					CreateV2: func() collectorapi.CollectorV2 {
						return &factoryTestV2{state: state}
					},
				},
			},
			Resolver:   resolver,
			StoreScope: unavailableStoreScope,
		},
	)
	if err != nil {
		b.Fatal(err)
	}
	config := factoryTestConfig(false)
	b.ReportAllocs()
	for b.Loop() {
		if err := factory.Validate(
			context.Background(),
			config,
		); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBJobGenerationLookup(b *testing.B) {
	generation := &JobGeneration{state: JobActive}
	b.ReportAllocs()
	for b.Loop() {
		if generation.State() != JobActive {
			b.Fatal("job generation state changed")
		}
	}
}
