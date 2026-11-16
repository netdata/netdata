// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestIndexDecodeTags(t *testing.T) {
	for name, tc := range map[string]struct {
		components, index, format, want string
		component                       uint
		invalid                         bool
	}{
		"first variable string":          {components: "[{type: octet_string}, {type: octet_string}, {type: integer}]", component: 1, index: "3.70.79.79.3.98.97.114.2", format: "octet_string", want: "FOO"},
		"second variable string":         {components: "[{type: octet_string}, {type: octet_string}, {type: integer}]", component: 2, index: "3.70.79.79.3.98.97.114.2", format: "octet_string", want: "bar"},
		"integer after strings":          {components: "[{type: octet_string}, {type: octet_string}, {type: integer}]", component: 3, index: "0.3.98.97.114.2", want: "2"},
		"empty string":                   {components: "[{type: integer}, {type: octet_string}]", component: 2, index: "7.0", format: "octet_string", want: ""},
		"utf8 and spaces":                {components: "[{type: octet_string}]", component: 1, index: "4.32.195.169.32", format: "octet_string", want: " é "},
		"fixed width":                    {components: "[{type: octet_string, length: 3}]", component: 1, index: "70.79.79", format: "octet_string", want: "FOO"},
		"legacy string format unchanged": {components: "[{type: octet_string, length: 3}]", component: 1, index: "70.79.79", format: "string", want: "70.79.79"},
		"truncated":                      {components: "[{type: octet_string}]", component: 1, index: "3.70.79", invalid: true},
		"extra suffix":                   {components: "[{type: integer}]", component: 1, index: "1.2", invalid: true},
		"oversized length":               {components: "[{type: octet_string}]", component: 1, index: "4294967295.70", invalid: true},
		"bad octet":                      {components: "[{type: octet_string}]", component: 1, index: "1.256", invalid: true},
		"negative integer":               {components: "[{type: integer}]", component: 1, index: "-1", invalid: true},
		"overflow integer":               {components: "[{type: integer}]", component: 1, index: "4294967296", invalid: true},
		"nondecimal integer":             {components: "[{type: integer}]", component: 1, index: "x", invalid: true},
		"invalid unselected component":   {components: "[{type: integer}, {type: octet_string}]", component: 1, index: "1.1.256", invalid: true},
		"bad formatted address":          {components: "[{type: octet_string}]", component: 1, index: "5.1.2.3.4.5", format: "ip_address", invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			def := decodeContractProfile(t, fmt.Sprintf("metrics:\n- table: {OID: 1.3.6.1.4.1.99999.880, name: rows}\n  symbols: [{OID: 1.3.6.1.4.1.99999.880.1, name: value}]\n  metric_tags:\n  - tag: identity\n    index_decode: {components: %s, component: %d}\n    symbol: {format: %q}\n", tc.components, tc.component, tc.format))
			_, got, err := newTableRowProcessor(logger.New()).processIndexTag(def.Metrics[0].MetricTags[0], tc.index)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIndexDecodeRejectedRowsDoNotEnterCache(t *testing.T) {
	for name, badIndex := range map[string]string{
		"malformed layout":            "1.5.192.0.2.1.7",
		"valid layout invalid format": "1.5.192.0.2.1.9.7",
	} {
		t.Run(name, func(t *testing.T) {
			const root = "1.3.6.1.4.1.99999.880"
			const goodIndex = "1.4.192.0.2.1.7"
			def := decodeContractProfile(t, "metrics:\n- table: {OID: "+root+", name: rows}\n  symbols: [{OID: "+root+".1, name: value}]\n  metric_tags:\n  - tag: address\n    index_decode: {components: [{type: integer}, {type: octet_string}, {type: integer}], component: 2}\n    symbol: {format: ip_address}\n")
			prof := &ddsnmp.Profile{SourceFile: "decode-test.yaml", Definition: def}
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			cache := newTableCache(time.Hour, 0)
			collector := newTableCollector(handler, make(map[string]bool), cache, logger.New(), false)
			expectSNMPWalk(handler, gosnmp.Version2c, root, []gosnmp.SnmpPDU{
				createGauge32PDU(root+".1."+goodIndex, 10), createGauge32PDU(root+".1."+badIndex, 20),
			})
			metrics, err := collector.collect(prof, &ddsnmp.CollectionStats{})
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.Equal(t, "192.0.2.1", metrics[0].Tags["address"])
			cachedOIDs, cachedTags, ok := cache.getCachedData(def.Metrics[0])
			require.True(t, ok)
			assert.NotContains(t, cachedOIDs, badIndex)
			assert.NotContains(t, cachedTags, badIndex)
			handler.EXPECT().Get([]string{root + ".1." + goodIndex}).Return(&gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{createGauge32PDU(root+".1."+goodIndex, 11)}}, nil)
			var stats ddsnmp.CollectionStats
			metrics, err = collector.collect(prof, &stats)
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.Equal(t, "192.0.2.1", metrics[0].Tags["address"])
			assert.EqualValues(t, 11, metrics[0].Value)
			assert.EqualValues(t, 1, stats.TableCache.Hits)
		})
	}
}

func TestIndexDecodeConfigChangeDoesNotReuseIdentity(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.880"
	def := decodeContractProfile(t, "metrics:\n- table: {OID: "+root+", name: rows}\n  symbols: [{OID: "+root+".1, name: value}]\n  metric_tags:\n  - tag: identity\n    index_decode: {components: [{type: integer}, {type: integer}], component: 1}\n")
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	collector := newTableCollector(handler, make(map[string]bool), newTableCache(time.Hour, 0), logger.New(), false)
	for _, component := range []uint{1, 2} {
		changed := def.Clone()
		changed.Metrics[0].MetricTags[0].IndexDecode.Component = component
		expectSNMPWalk(handler, gosnmp.Version2c, root, []gosnmp.SnmpPDU{createGauge32PDU(root+".1.1.2", 10)})
		metrics, err := collector.collect(&ddsnmp.Profile{SourceFile: "decode-test.yaml", Definition: changed}, &ddsnmp.CollectionStats{})
		require.NoError(t, err)
		require.Len(t, metrics, 1)
		assert.Equal(t, fmt.Sprint(component), metrics[0].Tags["identity"])
	}
}

func TestExtendedIndexTagConfigChangeDoesNotReuseIdentity(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.881"
	for _, tc := range []struct {
		name, before, after string
		want                map[string]string
	}{
		{
			name:   "fallback symbol name",
			before: "- symbol: {name: first}\n  index_decode: {components: [{type: integer}, {type: integer}], component: 1}\n",
			after:  "- symbol: {name: second}\n  index_decode: {components: [{type: integer}, {type: integer}], component: 1}\n",
			want:   map[string]string{"second": "65"},
		},
		{
			name:   "octet string index position",
			before: "- tag: identity\n  index: 1\n  symbol: {format: octet_string}\n",
			after:  "- tag: identity\n  index: 2\n  symbol: {format: octet_string}\n",
			want:   map[string]string{"identity": "B"},
		},
		{
			name:   "companion legacy tag",
			before: "- tag: identity\n  index: 1\n  symbol: {format: octet_string}\n- tag: first\n  index: 2\n",
			after:  "- tag: identity\n  index: 1\n  symbol: {format: octet_string}\n- tag: second\n  index: 2\n",
			want:   map[string]string{"identity": "A", "second": "66"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			collector := newTableCollector(handler, make(map[string]bool), newTableCache(time.Hour, 0), logger.New(), false)
			pdu := createGauge32PDU(root+".1.65.66", 10)
			for i, tags := range []string{tc.before, tc.after, tc.after} {
				def := decodeContractProfile(t, "metrics:\n- table: {OID: "+root+", name: rows}\n  symbols: [{OID: "+root+".1, name: value}]\n  metric_tags:\n    "+strings.ReplaceAll(tags, "\n", "\n    "))
				if i < 2 {
					expectSNMPWalk(handler, gosnmp.Version2c, root, []gosnmp.SnmpPDU{pdu})
				} else {
					handler.EXPECT().Get([]string{pdu.Name}).Return(&gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{pdu}}, nil)
				}
				metrics, err := collector.collect(&ddsnmp.Profile{SourceFile: "decode-test.yaml", Definition: def}, &ddsnmp.CollectionStats{})
				require.NoError(t, err)
				require.Len(t, metrics, 1)
				if i > 0 {
					assert.Equal(t, tc.want, metrics[0].Tags)
				}
			}
		})
	}
}
