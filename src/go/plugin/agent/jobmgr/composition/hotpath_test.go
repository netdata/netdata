package composition

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestModuleCatalogPolicyIsFrozenAtProcessConstruction(t *testing.T) {
	config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
	config.Modules = collectorapi.Registry{
		"enabled":  {Defaults: collectorapi.Defaults{UpdateEvery: 1}},
		"disabled": {Defaults: collectorapi.Defaults{UpdateEvery: 1, Disabled: true}},
	}
	config.Defaults = confgroup.Registry{"enabled": {UpdateEvery: 1}, "disabled": {UpdateEvery: 1}}
	process, err := NewProcess(config)
	require.NoError(t, err)
	delete(config.Modules, "enabled")
	config.Modules["late"] = collectorapi.Creator{}

	tests := map[string]struct {
		module       string
		wantFound    bool
		wantDisabled bool
	}{
		"enabled module remains frozen":  {module: "enabled", wantFound: true},
		"disabled policy remains frozen": {module: "disabled", wantFound: true, wantDisabled: true},
		"late mutation is absent":        {module: "late"},
		"unknown module is absent":       {module: "unknown"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			creator, found := process.core.config.Modules.Lookup(test.module)
			require.False(t, found != test.wantFound || (found && creator.Disabled != test.wantDisabled))
		})
	}
}

func TestModuleCatalogLookupAllocatesNothing(t *testing.T) {
	modules := collectorapi.Registry{"module": {Defaults: collectorapi.Defaults{UpdateEvery: 1}}}
	allocations := testing.AllocsPerRun(1_000, func() {
		if _, ok := modules.Lookup("module"); !ok {
			panic("module disappeared")
		}
	})
	require.EqualValues(t, 0, allocations)
}

func BenchmarkBProcessRestart(b *testing.B) {
	started := make(chan struct{})
	close(started)
	process := &Process{commands: make(chan processControl), started: started, done: make(chan struct{})}
	go func() {
		for control := range process.commands {
			control.result <- nil
		}
	}()
	b.Cleanup(func() {
		close(process.commands)
	})
	b.ReportAllocs()
	for b.Loop() {
		if err := process.Restart(context.Background()); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

func BenchmarkBModuleLookup(b *testing.B) {
	modules := collectorapi.Registry{"module": {Defaults: collectorapi.Defaults{UpdateEvery: 1}}}
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := modules.Lookup("module"); !ok {
			require.FailNow(b, "benchmark failed", "module disappeared")
		}
	}
}
