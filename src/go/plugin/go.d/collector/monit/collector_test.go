// SPDX-License-Identifier: GPL-3.0-or-later

package monit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataStatus, _ = os.ReadFile("testdata/v5.33.0/status.xml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataStatus":     dataStatus,
	} {
		require.NotNil(t, data, name)

	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success with default": {
			wantFail: false,
			config:   New().Config,
		},
		"fail when URL not set": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				},
			},
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
		wantFail bool
		prepare  func(t *testing.T) (monit *Collector, cleanup func())
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  caseOk,
		},
		"fail on unexpected XML response": {
			wantFail: true,
			prepare:  caseUnexpectedXMLResponse,
		},
		"fail on invalid data response": {
			wantFail: true,
			prepare:  caseInvalidDataResponse,
		},
		"fail on connection refused": {
			wantFail: true,
			prepare:  caseConnectionRefused,
		},
		"fail on 404 response": {
			wantFail: true,
			prepare:  case404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) (monit *Collector, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success on valid response": {
			prepare:         caseOk,
			wantNumOfCharts: len(baseCharts) + len(serviceCheckChartsTmpl)*25,
			wantMetrics: map[string]int64{
				"service_check_type_directory_name_directoryAlert_status_error":               1,
				"service_check_type_directory_name_directoryAlert_status_initializing":        0,
				"service_check_type_directory_name_directoryAlert_status_not_monitored":       0,
				"service_check_type_directory_name_directoryAlert_status_ok":                  0,
				"service_check_type_directory_name_directoryDisabled_status_error":            0,
				"service_check_type_directory_name_directoryDisabled_status_initializing":     0,
				"service_check_type_directory_name_directoryDisabled_status_not_monitored":    1,
				"service_check_type_directory_name_directoryDisabled_status_ok":               0,
				"service_check_type_directory_name_directoryNotExists_status_error":           1,
				"service_check_type_directory_name_directoryNotExists_status_initializing":    0,
				"service_check_type_directory_name_directoryNotExists_status_not_monitored":   0,
				"service_check_type_directory_name_directoryNotExists_status_ok":              0,
				"service_check_type_directory_name_directoryOk_status_error":                  0,
				"service_check_type_directory_name_directoryOk_status_initializing":           0,
				"service_check_type_directory_name_directoryOk_status_not_monitored":          0,
				"service_check_type_directory_name_directoryOk_status_ok":                     1,
				"service_check_type_file_name_fileAlert_status_error":                         1,
				"service_check_type_file_name_fileAlert_status_initializing":                  0,
				"service_check_type_file_name_fileAlert_status_not_monitored":                 0,
				"service_check_type_file_name_fileAlert_status_ok":                            0,
				"service_check_type_file_name_fileDisabled_status_error":                      0,
				"service_check_type_file_name_fileDisabled_status_initializing":               0,
				"service_check_type_file_name_fileDisabled_status_not_monitored":              1,
				"service_check_type_file_name_fileDisabled_status_ok":                         0,
				"service_check_type_file_name_fileNotExists_status_error":                     1,
				"service_check_type_file_name_fileNotExists_status_initializing":              0,
				"service_check_type_file_name_fileNotExists_status_not_monitored":             0,
				"service_check_type_file_name_fileNotExists_status_ok":                        0,
				"service_check_type_file_name_fileOk_status_error":                            0,
				"service_check_type_file_name_fileOk_status_initializing":                     0,
				"service_check_type_file_name_fileOk_status_not_monitored":                    0,
				"service_check_type_file_name_fileOk_status_ok":                               1,
				"service_check_type_filesystem_name_filesystemAlert_status_error":             1,
				"service_check_type_filesystem_name_filesystemAlert_status_initializing":      0,
				"service_check_type_filesystem_name_filesystemAlert_status_not_monitored":     0,
				"service_check_type_filesystem_name_filesystemAlert_status_ok":                0,
				"service_check_type_filesystem_name_filesystemDisabled_status_error":          0,
				"service_check_type_filesystem_name_filesystemDisabled_status_initializing":   0,
				"service_check_type_filesystem_name_filesystemDisabled_status_not_monitored":  1,
				"service_check_type_filesystem_name_filesystemDisabled_status_ok":             0,
				"service_check_type_filesystem_name_filesystemNotExists_status_error":         1,
				"service_check_type_filesystem_name_filesystemNotExists_status_initializing":  0,
				"service_check_type_filesystem_name_filesystemNotExists_status_not_monitored": 0,
				"service_check_type_filesystem_name_filesystemNotExists_status_ok":            0,
				"service_check_type_filesystem_name_filsystemOk_status_error":                 0,
				"service_check_type_filesystem_name_filsystemOk_status_initializing":          0,
				"service_check_type_filesystem_name_filsystemOk_status_not_monitored":         0,
				"service_check_type_filesystem_name_filsystemOk_status_ok":                    1,
				"service_check_type_host_name_hostAlert_status_error":                         1,
				"service_check_type_host_name_hostAlert_status_initializing":                  0,
				"service_check_type_host_name_hostAlert_status_not_monitored":                 0,
				"service_check_type_host_name_hostAlert_status_ok":                            0,
				"service_check_type_host_name_hostDisabled_status_error":                      0,
				"service_check_type_host_name_hostDisabled_status_initializing":               0,
				"service_check_type_host_name_hostDisabled_status_not_monitored":              1,
				"service_check_type_host_name_hostDisabled_status_ok":                         0,
				"service_check_type_host_name_hostNotExists_status_error":                     1,
				"service_check_type_host_name_hostNotExists_status_initializing":              0,
				"service_check_type_host_name_hostNotExists_status_not_monitored":             0,
				"service_check_type_host_name_hostNotExists_status_ok":                        0,
				"service_check_type_host_name_hostOk_status_error":                            0,
				"service_check_type_host_name_hostOk_status_initializing":                     0,
				"service_check_type_host_name_hostOk_status_not_monitored":                    0,
				"service_check_type_host_name_hostOk_status_ok":                               1,
				"service_check_type_network_name_networkAlert_status_error":                   1,
				"service_check_type_network_name_networkAlert_status_initializing":            0,
				"service_check_type_network_name_networkAlert_status_not_monitored":           0,
				"service_check_type_network_name_networkAlert_status_ok":                      0,
				"service_check_type_network_name_networkDisabled_status_error":                0,
				"service_check_type_network_name_networkDisabled_status_initializing":         0,
				"service_check_type_network_name_networkDisabled_status_not_monitored":        1,
				"service_check_type_network_name_networkDisabled_status_ok":                   0,
				"service_check_type_network_name_networkNotExists_status_error":               1,
				"service_check_type_network_name_networkNotExists_status_initializing":        0,
				"service_check_type_network_name_networkNotExists_status_not_monitored":       0,
				"service_check_type_network_name_networkNotExists_status_ok":                  0,
				"service_check_type_network_name_networkOk_status_error":                      0,
				"service_check_type_network_name_networkOk_status_initializing":               0,
				"service_check_type_network_name_networkOk_status_not_monitored":              0,
				"service_check_type_network_name_networkOk_status_ok":                         1,
				"service_check_type_process_name_processAlert_status_error":                   1,
				"service_check_type_process_name_processAlert_status_initializing":            0,
				"service_check_type_process_name_processAlert_status_not_monitored":           0,
				"service_check_type_process_name_processAlert_status_ok":                      0,
				"service_check_type_process_name_processDisabled_status_error":                0,
				"service_check_type_process_name_processDisabled_status_initializing":         0,
				"service_check_type_process_name_processDisabled_status_not_monitored":        1,
				"service_check_type_process_name_processDisabled_status_ok":                   0,
				"service_check_type_process_name_processNotExists_status_error":               1,
				"service_check_type_process_name_processNotExists_status_initializing":        0,
				"service_check_type_process_name_processNotExists_status_not_monitored":       0,
				"service_check_type_process_name_processNotExists_status_ok":                  0,
				"service_check_type_process_name_processOk_status_error":                      0,
				"service_check_type_process_name_processOk_status_initializing":               0,
				"service_check_type_process_name_processOk_status_not_monitored":              0,
				"service_check_type_process_name_processOk_status_ok":                         1,
				"service_check_type_system_name_pve-deb-work_status_error":                    0,
				"service_check_type_system_name_pve-deb-work_status_initializing":             0,
				"service_check_type_system_name_pve-deb-work_status_not_monitored":            0,
				"service_check_type_system_name_pve-deb-work_status_ok":                       1,
				"uptime": 33,
			},
		},
		"fail on unexpected XML response": {
			prepare:         caseUnexpectedXMLResponse,
			wantNumOfCharts: len(baseCharts),
			wantMetrics:     nil,
		},
		"fail on invalid data response": {
			prepare:         caseInvalidDataResponse,
			wantNumOfCharts: len(baseCharts),
			wantMetrics:     nil,
		},
		"fail on connection refused": {
			prepare:         caseConnectionRefused,
			wantNumOfCharts: len(baseCharts),
			wantMetrics:     nil,
		},
		"fail on 404 response": {
			prepare:         case404,
			wantNumOfCharts: len(baseCharts),
			wantMetrics:     nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			_ = collr.Check(context.Background())

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				assert.Equal(t, test.wantNumOfCharts, len(*collr.Charts()), "want number of charts")
			}
		})
	}
}

func caseOk(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != urlPathStatus || r.URL.RawQuery != urlQueryStatus {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(dataStatus)
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseUnexpectedXMLResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	data := `<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Status>
        <Code>200</Code>
        <Message>Success</Message>
    </Status>
    <Data>
        <User>
            <ID>12345</ID>
            <Name>John Doe</Name>
            <Email>johndoe@example.com</Email>
            <Roles>
                <Role>Admin</Role>
                <Role>User</Role>
            </Roles>
        </User>
        <Order>
            <OrderID>98765</OrderID>
            <Date>2024-08-15</Date>
            <Items>
                <Item>
                    <Name>Widget A</Name>
                    <Quantity>2</Quantity>
                    <Price>19.99</Price>
                </Item>
                <Item>
                    <Name>Gadget B</Name>
                    <Quantity>1</Quantity>
                    <Price>99.99</Price>
                </Item>
            </Items>
            <Total>139.97</Total>
        </Order>
    </Data>
</Response>
`
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(data))
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseInvalidDataResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:65001"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}

func case404(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}
