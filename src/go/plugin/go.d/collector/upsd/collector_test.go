// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) {
	collr := New()

	require.NotPanics(t, func() { collr.Cleanup(context.Background()) })

	mock := prepareMockConnOK()
	collr.newUpsdConn = func(Config) upsdConn { return mock }

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Collect(context.Background())
	require.NotPanics(t, func() { collr.Cleanup(context.Background()) })
	assert.True(t, mock.calledDisconnect)
}

func TestCollector_Init(t *testing.T) {
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareUpsd func() *Collector
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
			prepareUpsd: func() *Collector {
				collr := New()
				collr.Username = "user"
				collr.Password = "pass"
				return collr
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
			collr := test.prepareUpsd()
			collr.newUpsdConn = func(Config) upsdConn { return test.prepareMock() }

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.Charts())
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareUpsd          func() *Collector
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
				"ups_cp1600_battery.charge":          10000,
				"ups_cp1600_battery.runtime":         197000,
				"ups_cp1600_battery.voltage":         5480,
				"ups_cp1600_battery.voltage.nominal": 4800,
				"ups_cp1600_ups.status.BOOST":        0,
				"ups_cp1600_ups.status.BYPASS":       0,
				"ups_cp1600_ups.status.CAL":          0,
				"ups_cp1600_ups.status.CHRG":         0,
				"ups_cp1600_ups.status.DISCHRG":      0,
				"ups_cp1600_ups.status.FSD":          0,
				"ups_cp1600_ups.status.HB":           0,
				"ups_cp1600_ups.status.LB":           0,
				"ups_cp1600_ups.status.OB":           0,
				"ups_cp1600_ups.status.OFF":          0,
				"ups_cp1600_ups.status.OL":           1,
				"ups_cp1600_ups.status.OVER":         0,
				"ups_cp1600_ups.status.RB":           0,
				"ups_cp1600_ups.status.TRIM":         0,
				"ups_cp1600_ups.status.other":        0,
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
			wantCharts:           25,
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
			prepareUpsd: func() *Collector {
				collr := New()
				collr.Username = "user"
				collr.Password = "pass"
				return collr
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
			collr := test.prepareUpsd()
			require.NoError(t, collr.Init(context.Background()))

			mock := test.prepareMock()
			collr.newUpsdConn = func(Config) upsdConn { return mock }

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)

			assert.Equalf(t, test.wantCharts, len(*collr.Charts()), "number of charts")
			if len(test.wantCollected) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}

			assert.Equalf(t, test.wantConnConnect, mock.calledConnect, "calledConnect")
			assert.Equalf(t, test.wantConnDisconnect, mock.calledDisconnect, "calledDisconnect")
			assert.Equal(t, test.wantConnAuthenticate, mock.calledAuthenticate, "calledAuthenticate")
		})
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
		{
			name: "cp1600",
			vars: map[string]string{
				"battery.charge":                "100",
				"battery.charge.low":            "10",
				"battery.charge.warning":        "50",
				"battery.runtime":               "1970",
				"battery.runtime.low":           "150",
				"battery.type":                  "PbAc",
				"battery.voltage":               "54.8",
				"battery.voltage.nominal":       "48.0",
				"device.mfr":                    "American Power Conversion",
				"device.model":                  "Smart-UPS X 1500",
				"device.serial":                 "****************",
				"device.type":                   "ups",
				"driver.name":                   "usbhid-ups",
				"driver.parameter.bus":          "001",
				"driver.parameter.pollfreq":     "30",
				"driver.parameter.pollinterval": "2",
				"driver.parameter.port":         "auto",
				"driver.parameter.product":      "Smart-UPS X 1500 FW:UPS 09.8 / ID=20",
				"driver.parameter.productid":    "0003",
				"driver.parameter.serial":       "****************",
				"driver.parameter.synchronous":  "auto",
				"driver.parameter.vendor":       "American Power Conversion",
				"driver.parameter.vendorid":     "051D",
				"driver.version":                "2.8.0",
				"driver.version.data":           "APC HID 0.98",
				"driver.version.internal":       "0.47",
				"driver.version.usb":            "libusb-1.0.26 (API: 0x1000109)",
				"ups.beeper.status":             "enabled",
				"ups.delay.shutdown":            "20",
				"ups.firmware":                  "UPS 09.8 / ID=20",
				"ups.mfr":                       "American Power Conversion",
				"ups.mfr.date":                  "2019/01/12",
				"ups.model":                     "Smart-UPS X 1500",
				"ups.productid":                 "0003",
				"ups.serial":                    "****************",
				"ups.status":                    "OL",
				"ups.timer.reboot":              "-1",
				"ups.timer.shutdown":            "-1",
				"ups.vendorid":                  "051d",
			},
		},
	}

	return upsUnits, nil
}
