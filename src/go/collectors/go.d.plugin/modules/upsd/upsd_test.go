// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsd_Cleanup(t *testing.T) {
	upsd := New()

	require.NotPanics(t, upsd.Cleanup)

	mock := prepareMockConnOK()
	upsd.newUpsdConn = func(Config) upsdConn { return mock }

	require.True(t, upsd.Init())
	_ = upsd.Collect()
	require.NotPanics(t, upsd.Cleanup)
	assert.True(t, mock.calledDisconnect)
}

func TestUpsd_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			wantFail: false,
			config:   New().Config,
		},
		"fails when 'address' option not set": {
			wantFail: true,
			config:   Config{Address: ""},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			upsd := New()
			upsd.Config = test.config

			if test.wantFail {
				assert.False(t, upsd.Init())
			} else {
				assert.True(t, upsd.Init())
			}
		})
	}
}

func TestUpsd_Check(t *testing.T) {
	tests := map[string]struct {
		prepareUpsd func() *Upsd
		prepareMock func() *mockUpsdConn
		wantFail    bool
	}{
		"successful data collection": {
			wantFail:    false,
			prepareUpsd: New,
			prepareMock: prepareMockConnOK,
		},
		"error on connect()": {
			wantFail:    true,
			prepareUpsd: New,
			prepareMock: prepareMockConnErrOnConnect,
		},
		"error on authenticate()": {
			wantFail: true,
			prepareUpsd: func() *Upsd {
				upsd := New()
				upsd.Username = "user"
				upsd.Password = "pass"
				return upsd
			},
			prepareMock: prepareMockConnErrOnAuthenticate,
		},
		"error on upsList()": {
			wantFail:    true,
			prepareUpsd: New,
			prepareMock: prepareMockConnErrOnUpsUnits,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			upsd := test.prepareUpsd()
			upsd.newUpsdConn = func(Config) upsdConn { return test.prepareMock() }

			require.True(t, upsd.Init())

			if test.wantFail {
				assert.False(t, upsd.Check())
			} else {
				assert.True(t, upsd.Check())
			}
		})
	}
}

func TestUpsd_Charts(t *testing.T) {
	upsd := New()
	require.True(t, upsd.Init())
	assert.NotNil(t, upsd.Charts())
}

func TestUpsd_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareUpsd          func() *Upsd
		prepareMock          func() *mockUpsdConn
		wantCollected        map[string]int64
		wantCharts           int
		wantConnConnect      bool
		wantConnDisconnect   bool
		wantConnAuthenticate bool
	}{
		"successful data collection": {
			prepareUpsd: New,
			prepareMock: prepareMockConnOK,
			wantCollected: map[string]int64{
				"ups_cp1500_battery.charge":          10000,
				"ups_cp1500_battery.runtime":         489000,
				"ups_cp1500_battery.voltage":         2400,
				"ups_cp1500_battery.voltage.nominal": 2400,
				"ups_cp1500_input.voltage":           22700,
				"ups_cp1500_input.voltage.nominal":   23000,
				"ups_cp1500_output.voltage":          26000,
				"ups_cp1500_ups.load":                800,
				"ups_cp1500_ups.load.usage":          4300,
				"ups_cp1500_ups.realpower.nominal":   90000,
				"ups_cp1500_ups.status.BOOST":        0,
				"ups_cp1500_ups.status.BYPASS":       0,
				"ups_cp1500_ups.status.CAL":          0,
				"ups_cp1500_ups.status.CHRG":         0,
				"ups_cp1500_ups.status.DISCHRG":      0,
				"ups_cp1500_ups.status.FSD":          0,
				"ups_cp1500_ups.status.HB":           0,
				"ups_cp1500_ups.status.LB":           0,
				"ups_cp1500_ups.status.OB":           0,
				"ups_cp1500_ups.status.OFF":          0,
				"ups_cp1500_ups.status.OL":           1,
				"ups_cp1500_ups.status.OVER":         0,
				"ups_cp1500_ups.status.RB":           0,
				"ups_cp1500_ups.status.TRIM":         0,
				"ups_cp1500_ups.status.other":        0,
				"ups_pr3000_battery.charge":          10000,
				"ups_pr3000_battery.runtime":         110800,
				"ups_pr3000_battery.voltage":         5990,
				"ups_pr3000_battery.voltage.nominal": 4800,
				"ups_pr3000_input.voltage":           22500,
				"ups_pr3000_input.voltage.nominal":   23000,
				"ups_pr3000_output.voltage":          22500,
				"ups_pr3000_ups.load":                2800,
				"ups_pr3000_ups.load.usage":          84000,
				"ups_pr3000_ups.realpower.nominal":   300000,
				"ups_pr3000_ups.status.BOOST":        0,
				"ups_pr3000_ups.status.BYPASS":       0,
				"ups_pr3000_ups.status.CAL":          0,
				"ups_pr3000_ups.status.CHRG":         0,
				"ups_pr3000_ups.status.DISCHRG":      0,
				"ups_pr3000_ups.status.FSD":          0,
				"ups_pr3000_ups.status.HB":           0,
				"ups_pr3000_ups.status.LB":           0,
				"ups_pr3000_ups.status.OB":           0,
				"ups_pr3000_ups.status.OFF":          0,
				"ups_pr3000_ups.status.OL":           1,
				"ups_pr3000_ups.status.OVER":         0,
				"ups_pr3000_ups.status.RB":           0,
				"ups_pr3000_ups.status.TRIM":         0,
				"ups_pr3000_ups.status.other":        0,
			},
			wantCharts:           20,
			wantConnConnect:      true,
			wantConnDisconnect:   false,
			wantConnAuthenticate: false,
		},
		"error on connect()": {
			prepareUpsd:          New,
			prepareMock:          prepareMockConnErrOnConnect,
			wantCollected:        nil,
			wantCharts:           0,
			wantConnConnect:      true,
			wantConnDisconnect:   false,
			wantConnAuthenticate: false,
		},
		"error on authenticate()": {
			prepareUpsd: func() *Upsd {
				upsd := New()
				upsd.Username = "user"
				upsd.Password = "pass"
				return upsd
			},
			prepareMock:          prepareMockConnErrOnAuthenticate,
			wantCollected:        nil,
			wantCharts:           0,
			wantConnConnect:      true,
			wantConnDisconnect:   true,
			wantConnAuthenticate: true,
		},
		"err on upsList()": {
			prepareUpsd:          New,
			prepareMock:          prepareMockConnErrOnUpsUnits,
			wantCollected:        nil,
			wantCharts:           0,
			wantConnConnect:      true,
			wantConnDisconnect:   true,
			wantConnAuthenticate: false,
		},
		"command err on upsList() (unknown ups)": {
			prepareUpsd:          New,
			prepareMock:          prepareMockConnCommandErrOnUpsUnits,
			wantCollected:        nil,
			wantCharts:           0,
			wantConnConnect:      true,
			wantConnDisconnect:   false,
			wantConnAuthenticate: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			upsd := test.prepareUpsd()
			require.True(t, upsd.Init())

			mock := test.prepareMock()
			upsd.newUpsdConn = func(Config) upsdConn { return mock }

			mx := upsd.Collect()

			assert.Equal(t, test.wantCollected, mx)
			assert.Equalf(t, test.wantCharts, len(*upsd.Charts()), "number of charts")
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDims(t, upsd, mx)
			}
			assert.Equalf(t, test.wantConnConnect, mock.calledConnect, "calledConnect")
			assert.Equalf(t, test.wantConnDisconnect, mock.calledDisconnect, "calledDisconnect")
			assert.Equal(t, test.wantConnAuthenticate, mock.calledAuthenticate, "calledAuthenticate")
		})
	}
}

func ensureCollectedHasAllChartsDims(t *testing.T, upsd *Upsd, mx map[string]int64) {
	for _, chart := range *upsd.Charts() {
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

func prepareMockConnOK() *mockUpsdConn {
	return &mockUpsdConn{}
}

func prepareMockConnErrOnConnect() *mockUpsdConn {
	return &mockUpsdConn{errOnConnect: true}
}

func prepareMockConnErrOnAuthenticate() *mockUpsdConn {
	return &mockUpsdConn{errOnAuthenticate: true}
}

func prepareMockConnErrOnUpsUnits() *mockUpsdConn {
	return &mockUpsdConn{errOnUpsUnits: true}
}

func prepareMockConnCommandErrOnUpsUnits() *mockUpsdConn {
	return &mockUpsdConn{commandErrOnUpsUnits: true}
}

type mockUpsdConn struct {
	errOnConnect         bool
	errOnDisconnect      bool
	errOnAuthenticate    bool
	errOnUpsUnits        bool
	commandErrOnUpsUnits bool

	calledConnect      bool
	calledDisconnect   bool
	calledAuthenticate bool
}

func (m *mockUpsdConn) connect() error {
	m.calledConnect = true
	if m.errOnConnect {
		return errors.New("mock error on connect()")
	}
	return nil
}

func (m *mockUpsdConn) disconnect() error {
	m.calledDisconnect = true
	if m.errOnDisconnect {
		return errors.New("mock error on disconnect()")
	}
	return nil
}

func (m *mockUpsdConn) authenticate(_, _ string) error {
	m.calledAuthenticate = true
	if m.errOnAuthenticate {
		return errors.New("mock error on authenticate()")
	}
	return nil
}

func (m *mockUpsdConn) upsUnits() ([]upsUnit, error) {
	if m.errOnUpsUnits {
		return nil, errors.New("mock error on upsUnits()")
	}
	if m.commandErrOnUpsUnits {
		return nil, fmt.Errorf("%w: mock command error on upsUnits()", errUpsdCommand)
	}

	upsUnits := []upsUnit{
		{
			name: "pr3000",
			vars: map[string]string{
				"battery.charge":                "100",
				"battery.charge.warning":        "35",
				"battery.mfr.date":              "CPS",
				"battery.runtime":               "1108",
				"battery.runtime.low":           "300",
				"battery.type":                  "PbAcid",
				"battery.voltage":               "59.9",
				"battery.voltage.nominal":       "48",
				"device.mfr":                    "CPS",
				"device.model":                  "PR3000ERT2U",
				"device.serial":                 "P11MQ2000041",
				"device.type":                   "ups",
				"driver.name":                   "usbhid-ups",
				"driver.parameter.pollfreq":     "30",
				"driver.parameter.pollinterval": "2",
				"driver.parameter.port":         "auto",
				"driver.parameter.synchronous":  "no",
				"driver.version":                "2.7.4",
				"driver.version.data":           "CyberPower HID 0.4",
				"driver.version.internal":       "0.41",
				"input.voltage":                 "225.0",
				"input.voltage.nominal":         "230",
				"output.voltage":                "225.0",
				"ups.beeper.status":             "enabled",
				"ups.delay.shutdown":            "20",
				"ups.delay.start":               "30",
				"ups.load":                      "28",
				"ups.mfr":                       "CPS",
				"ups.model":                     "PR3000ERT2U",
				"ups.productid":                 "0601",
				"ups.realpower.nominal":         "3000",
				"ups.serial":                    "P11MQ2000041",
				"ups.status":                    "OL",
				"ups.test.result":               "No test initiated",
				"ups.timer.shutdown":            "0",
				"ups.timer.start":               "0",
				"ups.vendorid":                  "0764",
			},
		},
		{
			name: "cp1500",
			vars: map[string]string{
				"battery.charge":                "100",
				"battery.charge.low":            "10",
				"battery.charge.warning":        "20",
				"battery.mfr.date":              "CPS",
				"battery.runtime":               "4890",
				"battery.runtime.low":           "300",
				"battery.type":                  "PbAcid",
				"battery.voltage":               "24.0",
				"battery.voltage.nominal":       "24",
				"device.mfr":                    "CPS",
				"device.model":                  "CP1500EPFCLCD",
				"device.serial":                 "CRMNO2000312",
				"device.type":                   "ups",
				"driver.name":                   "usbhid-ups",
				"driver.parameter.bus":          "001",
				"driver.parameter.pollfreq":     "30",
				"driver.parameter.pollinterval": "2",
				"driver.parameter.port":         "auto",
				"driver.parameter.product":      "CP1500EPFCLCD",
				"driver.parameter.productid":    "0501",
				"driver.parameter.serial":       "CRMNO2000312",
				"driver.parameter.synchronous":  "no",
				"driver.parameter.vendor":       "CPS",
				"driver.parameter.vendorid":     "0764",
				"driver.version":                "2.7.4",
				"driver.version.data":           "CyberPower HID 0.4",
				"driver.version.internal":       "0.41",
				"input.transfer.high":           "260",
				"input.transfer.low":            "170",
				"input.voltage":                 "227.0",
				"input.voltage.nominal":         "230",
				"output.voltage":                "260.0",
				"ups.beeper.status":             "enabled",
				"ups.delay.shutdown":            "20",
				"ups.delay.start":               "30",
				"ups.load":                      "8",
				"ups.mfr":                       "CPS",
				"ups.model":                     "CP1500EPFCLCD",
				"ups.productid":                 "0501",
				"ups.realpower":                 "43",
				"ups.realpower.nominal":         "900",
				"ups.serial":                    "CRMNO2000312",
				"ups.status":                    "OL",
				"ups.test.result":               "No test initiated",
				"ups.timer.shutdown":            "-60",
				"ups.timer.start":               "-60",
				"ups.vendorid":                  "0764",
			},
		},
	}

	return upsUnits, nil
}
