// SPDX-License-Identifier: GPL-3.0-or-later

package fail2ban

import (
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

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

func TestFail2Ban_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Fail2Ban{}, dataConfigJSON, dataConfigYAML)
}

func TestFail2Ban_Init(t *testing.T) {
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
			f2b := New()
			f2b.Config = test.config

			if test.wantFail {
				assert.Error(t, f2b.Init())
			} else {
				assert.NoError(t, f2b.Init())
			}
		})
	}
}

func TestFail2Ban_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Fail2Ban
	}{
		"not initialized exec": {
			prepare: func() *Fail2Ban {
				return New()
			},
		},
		"after check": {
			prepare: func() *Fail2Ban {
				f2b := New()
				f2b.exec = prepareMockOk()
				_ = f2b.Check()
				return f2b
			},
		},
		"after collect": {
			prepare: func() *Fail2Ban {
				f2b := New()
				f2b.exec = prepareMockOk()
				_ = f2b.Collect()
				return f2b
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f2b := test.prepare()

			assert.NotPanics(t, f2b.Cleanup)
		})
	}
}

func TestFail2Ban_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestFail2Ban_Check(t *testing.T) {
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
			f2b := New()
			mock := test.prepareMock()
			f2b.exec = mock

			if test.wantFail {
				assert.Error(t, f2b.Check())
			} else {
				assert.NoError(t, f2b.Check())
			}
		})
	}
}

func TestFail2Ban_Collect(t *testing.T) {
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
			f2b := New()
			mock := test.prepareMock()
			f2b.exec = mock

			mx := f2b.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Len(t, *f2b.Charts(), len(jailChartsTmpl)*2)
				testMetricsHasAllChartsDims(t, f2b, mx)
			}
		})
	}
}

func testMetricsHasAllChartsDims(t *testing.T, f2b *Fail2Ban, mx map[string]int64) {
	for _, chart := range *f2b.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := mx[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
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
