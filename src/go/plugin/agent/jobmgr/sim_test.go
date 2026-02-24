// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wantExposedEntry struct {
	cfg    confgroup.Config
	status dyncfg.Status
}

type runSim struct {
	do func(mgr *Manager, in chan []*confgroup.Group)

	wantDiscovered []confgroup.Config
	wantSeen       []confgroup.Config
	wantExposed    []wantExposedEntry
	wantRunning    []string
	wantDyncfg     string
}

func (s *runSim) run(t *testing.T) {
	t.Helper()

	require.NotNil(t, s.do, "s.do is nil")

	var buf bytes.Buffer
	mgr := New()
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))
	mgr.Modules = prepareMockRegistry()

	done := make(chan struct{})
	grpCh := make(chan []*confgroup.Group)
	ctx, cancel := context.WithCancel(context.Background())

	go func() { defer close(done); defer close(grpCh); mgr.Run(ctx, grpCh) }()

	timeout := time.Second * 5

	select {
	case <-mgr.started:
	case <-time.After(timeout):
		t.Errorf("failed to start work in %s", timeout)
	}

	s.do(mgr, grpCh)
	cancel()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Errorf("failed to finish work in %s", timeout)
	}

	var lines []string
	for _, s := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(s, "CONFIG") && strings.Contains(s, " template ") {
			continue
		}
		if strings.HasPrefix(s, "FUNCTION GLOBAL") {
			continue
		}
		if strings.HasPrefix(s, "FUNCTION_RESULT_BEGIN") {
			parts := strings.Fields(s)
			s = strings.Join(parts[:len(parts)-1], " ") // remove timestamp
		}
		lines = append(lines, s)
	}
	wantDyncfg, gotDyncfg := strings.TrimSpace(s.wantDyncfg), strings.TrimSpace(strings.Join(lines, "\n"))

	//fmt.Println(gotDyncfg)

	assert.Equal(t, wantDyncfg, gotDyncfg, "dyncfg commands")

	var n int
	for _, cfgs := range mgr.discoveredConfigs.items {
		n += len(cfgs)
	}

	wantLen, gotLen := len(s.wantDiscovered), n
	require.Equalf(t, wantLen, gotLen, "discoveredConfigs: different len (want %d got %d)", wantLen, gotLen)

	for _, cfg := range s.wantDiscovered {
		cfgs, ok := mgr.discoveredConfigs.items[cfg.Source()]
		require.Truef(t, ok, "discoveredConfigs: source %s is not found", cfg.Source())
		_, ok = cfgs[cfg.Hash()]
		require.Truef(t, ok, "discoveredConfigs: source %s config %d is not found", cfg.Source(), cfg.Hash())
	}

	wantLen, gotLen = len(s.wantSeen), mgr.seen.Count()
	require.Equalf(t, wantLen, gotLen, "seen: different len (want %d got %d)", wantLen, gotLen)

	for _, cfg := range s.wantSeen {
		_, ok := mgr.seen.Lookup(cfg)
		require.Truef(t, ok, "seen: config '%s' is not found", cfg.UID())
	}

	wantLen, gotLen = len(s.wantExposed), mgr.exposed.Count()
	require.Equalf(t, wantLen, gotLen, "exposed: different len (want %d got %d)", wantLen, gotLen)

	for _, we := range s.wantExposed {
		entry, ok := mgr.exposed.LookupByKey(we.cfg.ExposedKey())
		require.Truef(t, ok && we.cfg.UID() == entry.Cfg.UID(), "exposed: config '%s' is not found", we.cfg.UID())
		require.Truef(t, we.status == entry.Status, "exposed: wrong status for '%s', want %s got %s", we.cfg.UID(), we.status, entry.Status)
	}

	wantLen, gotLen = len(s.wantRunning), len(mgr.runningJobs.items)
	require.Equalf(t, wantLen, gotLen, "runningJobs: different len (want %d got %d)", wantLen, gotLen)
	for _, name := range s.wantRunning {
		_, ok := mgr.runningJobs.lookup(name)
		require.Truef(t, ok, "runningJobs: job '%s' is not found", name)
	}
}

func prepareMockRegistry() collectorapi.Registry {
	reg := collectorapi.Registry{}
	type config struct {
		OptionOne string `yaml:"option_one" json:"option_one"`
		OptionTwo int64  `yaml:"option_two" json:"option_two"`
	}

	reg.Register("success", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "title", Units: "units", Dims: collectorapi.Dims{{ID: "id1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"id1": 1} },
			}
		},
		Config: func() any {
			return &config{OptionOne: "one", OptionTwo: 2}
		},
	})
	reg.Register("fail", collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error { return errors.New("mock failed init") },
			}
		},
	})

	// CollectorV1 without Methods - for testing function_only config rejection
	reg.Register("nofuncs", collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "title", Units: "units", Dims: collectorapi.Dims{{ID: "id1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"id1": 1} },
			}
		},
	})

	// CollectorV1 with Methods - for testing config-level function_only
	reg.Register("withfuncs", collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "title", Units: "units", Dims: collectorapi.Dims{{ID: "id1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"id1": 1} },
			}
		},
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "test-method", Name: "Test Method"}}
		},
	})

	// FunctionOnly module - for testing module-level function-only
	reg.Register("funconly", collectorapi.Creator{
		FunctionOnly: true,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				ChartsFunc:  func() *collectorapi.Charts { return nil },
				CollectFunc: func(context.Context) map[string]int64 { return nil },
			}
		},
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "test-method", Name: "Test Method"}}
		},
	})

	return reg
}
