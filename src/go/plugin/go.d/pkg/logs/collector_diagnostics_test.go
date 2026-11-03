// SPDX-License-Identifier: GPL-3.0-or-later

package logs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/squidlog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/weblog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Both consumers must keep source contents out of diagnostics while preserving
// their existing unmatched-line and rejected-dimension behavior.
func TestCollectorSourceDiagnostics(t *testing.T) {
	const marker = "SYNTHETIC_SOURCE_MARKER"
	previous := slog.LevelDebug
	for !logger.Level.Enabled(previous) {
		previous++
	}
	logger.Level.Set(slog.LevelDebug)
	t.Cleanup(func() { logger.Level.Set(previous) })

	for name, consumer := range map[string]struct {
		newCollector                              func() collectorapi.CollectorV1
		valid, unused, assignment, verification   string
		invalidJSON, validLTSV                    string
		unmatchedMetric, unmatchedLog, chart, dim string
	}{
		"weblog": {
			newCollector:    func() collectorapi.CollectorV1 { return weblog.New() },
			valid:           `{"status":200,"host":"example.test"}` + "\n",
			unused:          `{"status":200,"host":"example.test","ignored":"SYNTHETIC_SOURCE_MARKER"}` + "\n",
			assignment:      `{"status":"SYNTHETIC_SOURCE_MARKER"}` + "\n",
			verification:    `{"status":200,"host":"SYNTHETIC_SOURCE_MARKER invalid"}` + "\n",
			invalidJSON:     `{"status":SYNTHETIC_SOURCE_MARKER}` + "\n",
			validLTSV:       "status:200\n",
			unmatchedMetric: "req_unmatched",
			unmatchedLog:    "unmatched line",
			chart:           "requests_by_vhost",
			dim:             "req_vhost_" + marker + " invalid",
		},
		"squidlog": {
			newCollector:    func() collectorapi.CollectorV1 { return squidlog.New() },
			valid:           `{"resp_time":1,"server_address":"192.0.2.1"}` + "\n",
			unused:          `{"resp_time":1,"server_address":"192.0.2.1","ignored":"SYNTHETIC_SOURCE_MARKER"}` + "\n",
			assignment:      `{"resp_time":"SYNTHETIC_SOURCE_MARKER"}` + "\n",
			verification:    `{"resp_time":1,"server_address":"SYNTHETIC_SOURCE_MARKER invalid"}` + "\n",
			invalidJSON:     `{"resp_time":SYNTHETIC_SOURCE_MARKER}` + "\n",
			validLTSV:       "resp_time:1\n",
			unmatchedMetric: "unmatched",
			chart:           "forwarded_requests_by_server_address",
			dim:             "server_address_" + marker + " invalid",
		},
	} {
		t.Run(name, func(t *testing.T) {
			for name, test := range map[string]struct {
				kind                                string
				initial, appended, wantLog          string
				wantNil, wantUnmatched, rejectedDim bool
			}{
				"startup unused sample value": {
					kind: logs.TypeJSON, initial: consumer.unused, wantLog: "starting parser creating",
				},
				"startup assignment": {
					kind: logs.TypeJSON, initial: consumer.assignment + consumer.valid, wantLog: "failed to parse line",
				},
				"startup verification": {
					kind: logs.TypeJSON, initial: consumer.verification + consumer.valid, wantLog: "failed to verify line",
				},
				"later assignment": {
					kind: logs.TypeJSON, initial: consumer.valid, appended: consumer.assignment,
					wantLog: consumer.unmatchedLog, wantUnmatched: true,
				},
				"later JSON syntax": {
					kind: logs.TypeJSON, initial: consumer.valid, appended: consumer.invalidJSON,
					wantLog: "json parse", wantNil: true,
				},
				"later LTSV syntax": {
					kind: logs.TypeLTSV, initial: consumer.validLTSV, appended: marker + "\n",
					wantLog: consumer.unmatchedLog, wantUnmatched: true,
				},
				"later rejected dimension": {
					kind: logs.TypeJSON, initial: consumer.valid, appended: consumer.verification,
					wantLog: "level=warn", rejectedDim: true,
				},
			} {
				t.Run(name, func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "access.log")
					require.NoError(t, os.WriteFile(path, []byte(test.initial), 0600))
					var output bytes.Buffer
					collector := consumer.newCollector()
					collector.GetBase().Logger = logger.NewWithWriter(&output)
					config, err := json.Marshal(map[string]string{"path": path, "log_type": test.kind})
					require.NoError(t, err)
					require.NoError(t, json.Unmarshal(config, collector))
					t.Cleanup(func() { collector.Cleanup(context.Background()) })
					require.NoError(t, collector.Init(t.Context()))
					require.NoError(t, collector.Check(t.Context()))
					if test.appended != "" {
						output.Reset()
						file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
						require.NoError(t, err)
						_, err = file.WriteString(test.appended)
						require.NoError(t, err)
						require.NoError(t, file.Close())
						metrics := collector.Collect(t.Context())
						if test.wantNil {
							assert.Nil(t, metrics)
						} else {
							require.NotNil(t, metrics)
							assert.Equal(t, int64(1), metrics["requests"])
							if test.wantUnmatched {
								assert.Equal(t, int64(1), metrics[consumer.unmatchedMetric])
							}
						}
						if test.rejectedDim {
							chart := collector.Charts().Get(consumer.chart)
							require.NotNil(t, chart)
							assert.Nil(t, chart.GetDim(consumer.dim))
						}
					}
					assert.Contains(t, output.String(), test.wantLog)
					assert.NotContains(t, output.String(), marker)
				})
			}
		})
	}
}
