// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runSim struct {
	do func(mgr *Manager, in chan []*confgroup.Group)

	wantDiscovered []confgroup.Config
	wantSeen       []seenConfig
	wantExposed    []seenConfig
	wantRunning    []string
	wantDyncfg     string
}

func (s *runSim) run(t *testing.T) {
	t.Helper()

	require.NotNil(t, s.do, "s.do is nil")

	var buf bytes.Buffer
	mgr := New()
	mgr.dyncfgApi = dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf)))
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

	wantLen, gotLen = len(s.wantSeen), len(mgr.seenConfigs.items)
	require.Equalf(t, wantLen, gotLen, "seenConfigs: different len (want %d got %d)", wantLen, gotLen)

	for _, scfg := range s.wantSeen {
		v, ok := mgr.seenConfigs.lookup(scfg.cfg)
		require.Truef(t, ok, "seenConfigs: config '%s' is not found", scfg.cfg.UID())
		require.Truef(t, scfg.status == v.status, "seenConfigs: wrong status, want %s got %s", scfg.status, v.status)
	}

	wantLen, gotLen = len(s.wantExposed), len(mgr.exposedConfigs.items)
	require.Equalf(t, wantLen, gotLen, "exposedConfigs: different len (want %d got %d)", wantLen, gotLen)

	for _, scfg := range s.wantExposed {
		v, ok := mgr.exposedConfigs.lookup(scfg.cfg)
		require.Truef(t, ok && scfg.cfg.UID() == v.cfg.UID(), "exposedConfigs: config '%s' is not found", scfg.cfg.UID())
		require.Truef(t, scfg.status == v.status, "exposedConfigs: wrong status, want %s got %s", scfg.status, v.status)
	}

	wantLen, gotLen = len(s.wantRunning), len(mgr.runningJobs.items)
	require.Equalf(t, wantLen, gotLen, "runningJobs: different len (want %d got %d)", wantLen, gotLen)
	for _, name := range s.wantRunning {
		_, ok := mgr.runningJobs.lookup(name)
		require.Truef(t, ok, "runningJobs: job '%s' is not found", name)
	}
}

func prepareMockRegistry() module.Registry {
	reg := module.Registry{}
	type config struct {
		OptionOne string `yaml:"option_one" json:"option_one"`
		OptionTwo int64  `yaml:"option_two" json:"option_two"`
	}

	reg.Register("success", module.Creator{
		JobConfigSchema: module.MockConfigSchema,
		Create: func() module.Module {
			return &module.MockModule{
				ChartsFunc: func() *module.Charts {
					return &module.Charts{&module.Chart{ID: "id", Title: "title", Units: "units", Dims: module.Dims{{ID: "id1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"id1": 1} },
			}
		},
		Config: func() any {
			return &config{OptionOne: "one", OptionTwo: 2}
		},
	})
	reg.Register("fail", module.Creator{
		Create: func() module.Module {
			return &module.MockModule{
				InitFunc: func(context.Context) error { return errors.New("mock failed init") },
			}
		},
	})

	return reg
}
