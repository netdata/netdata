// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/agent/functions"
	"github.com/netdata/go.d.plugin/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDiscovery(t *testing.T) {

}

func TestDiscovery_Register(t *testing.T) {
	tests := map[string]struct {
		regConfigs   []confgroup.Config
		wantApiStats *mockApi
		wantConfigs  int
	}{
		"register jobs created by Dyncfg and other providers": {
			regConfigs: []confgroup.Config{
				prepareConfig(
					"__provider__", dynCfg,
					"module", "test",
					"name", "first",
				),
				prepareConfig(
					"__provider__", "test",
					"module", "test",
					"name", "second",
				),
			},
			wantConfigs: 2,
			wantApiStats: &mockApi{
				callsDynCfgRegisterJob: 1,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var mock mockApi
			d := &Discovery{
				API:     &mock,
				mux:     &sync.Mutex{},
				configs: make(map[string]confgroup.Config),
			}

			for _, v := range test.regConfigs {
				d.Register(v)
			}

			assert.Equal(t, test.wantApiStats, &mock)
			assert.Equal(t, test.wantConfigs, len(d.configs))
		})
	}
}

func TestDiscovery_Unregister(t *testing.T) {
	tests := map[string]struct {
		regConfigs   []confgroup.Config
		unregConfigs []confgroup.Config
		wantApiStats *mockApi
		wantConfigs  int
	}{
		"register/unregister jobs created by Dyncfg and other providers": {
			wantConfigs: 0,
			wantApiStats: &mockApi{
				callsDynCfgRegisterJob: 1,
			},
			regConfigs: []confgroup.Config{
				prepareConfig(
					"__provider__", dynCfg,
					"module", "test",
					"name", "first",
				),
				prepareConfig(
					"__provider__", "test",
					"module", "test",
					"name", "second",
				),
			},
			unregConfigs: []confgroup.Config{
				prepareConfig(
					"__provider__", dynCfg,
					"module", "test",
					"name", "first",
				),
				prepareConfig(
					"__provider__", "test",
					"module", "test",
					"name", "second",
				),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var mock mockApi
			d := &Discovery{
				API:     &mock,
				mux:     &sync.Mutex{},
				configs: make(map[string]confgroup.Config),
			}

			for _, v := range test.regConfigs {
				d.Register(v)
			}
			for _, v := range test.unregConfigs {
				d.Unregister(v)
			}

			assert.Equal(t, test.wantApiStats, &mock)
			assert.Equal(t, test.wantConfigs, len(d.configs))
		})
	}
}

func TestDiscovery_UpdateStatus(t *testing.T) {

}

func TestDiscovery_Run(t *testing.T) {
	tests := map[string]struct {
		wantApiStats *mockApi
	}{
		"default run": {
			wantApiStats: &mockApi{
				callsDynCfgEnable:          1,
				callsDyncCfgRegisterModule: 2,
				callsRegister:              10,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var mock mockApi
			d, err := NewDiscovery(Config{
				Plugin:    "test",
				API:       &mock,
				Functions: &mock,
				Modules: module.Registry{
					"module1": module.Creator{},
					"module2": module.Creator{},
				},
				ModuleConfigDefaults: nil,
			})
			require.Nil(t, err)

			testTime := time.Second * 3
			ctx, cancel := context.WithTimeout(context.Background(), testTime)
			defer cancel()

			in := make(chan<- []*confgroup.Group)
			done := make(chan struct{})

			go func() { defer close(done); d.Run(ctx, in) }()

			timeout := testTime + time.Second*2
			tk := time.NewTimer(timeout)
			defer tk.Stop()

			select {
			case <-done:
				assert.Equal(t, test.wantApiStats, &mock)
			case <-tk.C:
				t.Errorf("timed out after %s", timeout)
			}
		})
	}
}

type mockApi struct {
	callsDynCfgEnable          int
	callsDyncCfgRegisterModule int
	callsDynCfgRegisterJob     int
	callsDynCfgReportJobStatus int
	callsFunctionResultSuccess int
	callsFunctionResultReject  int

	callsRegister int
}

func (m *mockApi) Register(string, func(functions.Function)) {
	m.callsRegister++
}

func (m *mockApi) DynCfgEnable(string) error {
	m.callsDynCfgEnable++
	return nil
}

func (m *mockApi) DynCfgReset() error {
	return nil
}

func (m *mockApi) DyncCfgRegisterModule(string) error {
	m.callsDyncCfgRegisterModule++
	return nil
}

func (m *mockApi) DynCfgRegisterJob(_, _, _ string) error {
	m.callsDynCfgRegisterJob++
	return nil
}

func (m *mockApi) DynCfgReportJobStatus(_, _, _, _ string) error {
	m.callsDynCfgReportJobStatus++
	return nil
}

func (m *mockApi) FunctionResultSuccess(_, _, _ string) error {
	m.callsFunctionResultSuccess++
	return nil
}

func (m *mockApi) FunctionResultReject(_, _, _ string) error {
	m.callsFunctionResultReject++
	return nil
}

func prepareConfig(values ...string) confgroup.Config {
	cfg := confgroup.Config{}
	for i := 1; i < len(values); i += 2 {
		cfg[values[i-1]] = values[i]
	}
	return cfg
}
