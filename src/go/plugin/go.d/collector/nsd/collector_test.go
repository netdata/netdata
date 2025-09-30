// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

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

	dataStats, _ = os.ReadFile("testdata/stats.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataStats":      dataStats,
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
		"success with default config": {
			wantFail: false,
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
				collr.exec = prepareMockOK()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOK()
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
		prepareMock func() *mockNsdControl
		wantFail    bool
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantFail:    false,
		},
		"error on stats call": {
			prepareMock: prepareMockErrOnStats,
			wantFail:    true,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantFail:    true,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
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
		prepareMock func() *mockNsdControl
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantMetrics: map[string]int64{
				"num.answer_wo_aa":    1,
				"num.class.CH":        0,
				"num.class.CS":        0,
				"num.class.HS":        0,
				"num.class.IN":        1,
				"num.dropped":         1,
				"num.edns":            1,
				"num.ednserr":         1,
				"num.opcode.IQUERY":   0,
				"num.opcode.NOTIFY":   0,
				"num.opcode.OTHER":    0,
				"num.opcode.QUERY":    1,
				"num.opcode.STATUS":   0,
				"num.opcode.UPDATE":   0,
				"num.queries":         1,
				"num.raxfr":           1,
				"num.rcode.BADVERS":   0,
				"num.rcode.FORMERR":   1,
				"num.rcode.NOERROR":   1,
				"num.rcode.NOTAUTH":   0,
				"num.rcode.NOTIMP":    1,
				"num.rcode.NOTZONE":   0,
				"num.rcode.NXDOMAIN":  1,
				"num.rcode.NXRRSET":   0,
				"num.rcode.RCODE11":   0,
				"num.rcode.RCODE12":   0,
				"num.rcode.RCODE13":   0,
				"num.rcode.RCODE14":   0,
				"num.rcode.RCODE15":   0,
				"num.rcode.REFUSED":   1,
				"num.rcode.SERVFAIL":  1,
				"num.rcode.YXDOMAIN":  1,
				"num.rcode.YXRRSET":   0,
				"num.rixfr":           1,
				"num.rxerr":           1,
				"num.tcp":             1,
				"num.tcp6":            1,
				"num.tls":             1,
				"num.tls6":            1,
				"num.truncated":       1,
				"num.txerr":           1,
				"num.type.A":          1,
				"num.type.AAAA":       1,
				"num.type.AFSDB":      1,
				"num.type.APL":        1,
				"num.type.AVC":        0,
				"num.type.CAA":        0,
				"num.type.CDNSKEY":    1,
				"num.type.CDS":        1,
				"num.type.CERT":       1,
				"num.type.CNAME":      1,
				"num.type.CSYNC":      1,
				"num.type.DHCID":      1,
				"num.type.DLV":        0,
				"num.type.DNAME":      1,
				"num.type.DNSKEY":     1,
				"num.type.DS":         1,
				"num.type.EUI48":      1,
				"num.type.EUI64":      1,
				"num.type.HINFO":      1,
				"num.type.HTTPS":      1,
				"num.type.IPSECKEY":   1,
				"num.type.ISDN":       1,
				"num.type.KEY":        1,
				"num.type.KX":         1,
				"num.type.L32":        1,
				"num.type.L64":        1,
				"num.type.LOC":        1,
				"num.type.LP":         1,
				"num.type.MB":         1,
				"num.type.MD":         1,
				"num.type.MF":         1,
				"num.type.MG":         1,
				"num.type.MINFO":      1,
				"num.type.MR":         1,
				"num.type.MX":         1,
				"num.type.NAPTR":      1,
				"num.type.NID":        1,
				"num.type.NS":         1,
				"num.type.NSAP":       1,
				"num.type.NSEC":       1,
				"num.type.NSEC3":      1,
				"num.type.NSEC3PARAM": 1,
				"num.type.NULL":       1,
				"num.type.NXT":        1,
				"num.type.OPENPGPKEY": 1,
				"num.type.OPT":        1,
				"num.type.PTR":        1,
				"num.type.PX":         1,
				"num.type.RP":         1,
				"num.type.RRSIG":      1,
				"num.type.RT":         1,
				"num.type.SIG":        1,
				"num.type.SMIMEA":     1,
				"num.type.SOA":        1,
				"num.type.SPF":        1,
				"num.type.SRV":        1,
				"num.type.SSHFP":      1,
				"num.type.SVCB":       1,
				"num.type.TLSA":       1,
				"num.type.TXT":        1,
				"num.type.TYPE252":    0,
				"num.type.TYPE255":    0,
				"num.type.URI":        0,
				"num.type.WKS":        1,
				"num.type.X25":        1,
				"num.type.ZONEMD":     1,
				"num.udp":             1,
				"num.udp6":            1,
				"server0.queries":     1,
				"size.config.disk":    1,
				"size.config.mem":     1064,
				"size.db.disk":        576,
				"size.db.mem":         920,
				"size.xfrd.mem":       1160464,
				"time.boot":           556,
				"zone.master":         1,
				"zone.slave":          1,
			},
		},
		"error on lvs report call": {
			prepareMock: prepareMockErrOnStats,
			wantMetrics: nil,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
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
				assert.Len(t, *collr.Charts(), len(charts))
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareMockOK() *mockNsdControl {
	return &mockNsdControl{
		dataStats: dataStats,
	}
}

func prepareMockErrOnStats() *mockNsdControl {
	return &mockNsdControl{
		errOnStatus: true,
	}
}

func prepareMockEmptyResponse() *mockNsdControl {
	return &mockNsdControl{}
}

func prepareMockUnexpectedResponse() *mockNsdControl {
	return &mockNsdControl{
		dataStats: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockNsdControl struct {
	errOnStatus bool
	dataStats   []byte
}

func (m *mockNsdControl) stats() ([]byte, error) {
	if m.errOnStatus {
		return nil, errors.New("mock.status() error")
	}
	return m.dataStats, nil
}
