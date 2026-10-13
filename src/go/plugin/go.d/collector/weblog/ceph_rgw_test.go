// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"bytes"
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/stretchr/testify/require"
)

func TestCollector_CephRGWAccessLogParsesTotalTimeMilliseconds(t *testing.T) {
	record := []byte(`{"remote_addr":"192.0.2.10","uri":"GET /bucket/object HTTP/1.1","http_status":"200",` +
		`"bytes_sent":"1024","total_time":"17"}` + "\n")

	cfg := Config{
		ParserConfig: logs.ParserConfig{
			LogType: logs.TypeJSON,
			JSON: logs.JSONConfig{
				Mapping: map[string]string{
					"remote_addr": "remote_addr",
					"uri":         "request",
					"http_status": "status",
					"bytes_sent":  "bytes_sent",
					"total_time":  "total_time",
				},
			},
		},
		CustomNumericFields: []customNumericField{{Name: "total_time", Units: "milliseconds"}},
		Path:                "testdata/ceph_rgw_access.json",
		ExcludePath:         "*.gz",
	}
	collr := New()
	collr.Config = cfg
	require.NoError(t, collr.Init(context.Background()))
	defer collr.Cleanup(context.Background())

	parser, err := logs.NewJSONParser(collr.ParserConfig.JSON, bytes.NewReader(record))
	require.NoError(t, err)

	collr.line.reset()
	require.NoError(t, parser.Parse(record, collr.line))

	var total string
	for _, custom := range collr.line.custom.values {
		if custom.name == "total_time" {
			total = custom.value
		}
	}
	require.Equal(t, "17", total)
	require.Contains(t, collr.customNumericFields, "total_time")
}
