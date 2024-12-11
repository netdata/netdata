// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataStatus, _     = os.ReadFile("testdata/fail2ban-status.txt")
	dataJailStatus, _ = os.ReadFile("testdata/fail2ban-jail-status.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataStatus":     dataStatus,
		"dataJailStatus": dataJailStatus,
	} {
		require.NotNil(t, data, name)

	}
}

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if failed to locate ndsudo": {
			wantFail: true,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"not initialized exec": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOk()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOk()
				_ = collr.Collect(context.Background())
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockFail2BanClientCliExec
		wantFail    bool
	}{
		"success multiple jails": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"error on status": {
			wantFail:    true,
			prepareMock: prepareMockErrOnStatus,
		},
		"empty response (no jails)": {
			prepareMock: prepareMockEmptyResponse,
			wantFail:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockFail2BanClientCliExec
		wantMetrics map[string]int64
	}{
		"success multiple jails": {
			prepareMock: prepareMockOk,
			wantMetrics: map[string]int64{
				"jail_dovecot_currently_banned": 30,
				"jail_dovecot_currently_failed": 10,
				"jail_sshd_currently_banned":    30,
				"jail_sshd_currently_failed":    10,
			},
		},
		"error on status": {
			prepareMock: prepareMockErrOnStatus,
			wantMetrics: nil,
		},
		"empty response (no jails)": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Len(t, *collr.Charts(), len(jailChartsTmpl)*2, "wantCharts")

				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareMockOk() *mockFail2BanClientCliExec {
	return &mockFail2BanClientCliExec{
		statusData:     dataStatus,
		jailStatusData: dataJailStatus,
	}
}

func prepareMockErrOnStatus() *mockFail2BanClientCliExec {
	return &mockFail2BanClientCliExec{
		errOnStatus:    true,
		statusData:     dataStatus,
		jailStatusData: dataJailStatus,
	}
}

func prepareMockEmptyResponse() *mockFail2BanClientCliExec {
	return &mockFail2BanClientCliExec{}
}

type mockFail2BanClientCliExec struct {
	errOnStatus bool
	statusData  []byte

	errOnJailStatus bool
	jailStatusData  []byte
}

func (m *mockFail2BanClientCliExec) status() ([]byte, error) {
	if m.errOnStatus {
		return nil, errors.New("mock.status() error")
	}

	return m.statusData, nil
}

func (m *mockFail2BanClientCliExec) jailStatus(_ string) ([]byte, error) {
	if m.errOnJailStatus {
		return nil, errors.New("mock.jailStatus() error")
	}

	return m.jailStatusData, nil
}
