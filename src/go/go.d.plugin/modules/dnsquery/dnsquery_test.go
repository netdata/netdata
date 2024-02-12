// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	"errors"
	"testing"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	assert.Implements(t, (*module.Module)(nil), New())
}

func TestDNSQuery_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success when all set": {
			wantFail: false,
			config: Config{
				Domains:     []string{"example.com"},
				Servers:     []string{"192.0.2.0"},
				Network:     "udp",
				RecordTypes: []string{"A"},
				Port:        53,
				Timeout:     web.Duration{Duration: time.Second},
			},
		},
		"success when using deprecated record_type": {
			wantFail: false,
			config: Config{
				Domains:    []string{"example.com"},
				Servers:    []string{"192.0.2.0"},
				Network:    "udp",
				RecordType: "A",
				Port:       53,
				Timeout:    web.Duration{Duration: time.Second},
			},
		},
		"fail with default": {
			wantFail: true,
			config:   New().Config,
		},
		"fail when domains not set": {
			wantFail: true,
			config: Config{
				Domains:     nil,
				Servers:     []string{"192.0.2.0"},
				Network:     "udp",
				RecordTypes: []string{"A"},
				Port:        53,
				Timeout:     web.Duration{Duration: time.Second},
			},
		},
		"fail when servers not set": {
			wantFail: true,
			config: Config{
				Domains:     []string{"example.com"},
				Servers:     nil,
				Network:     "udp",
				RecordTypes: []string{"A"},
				Port:        53,
				Timeout:     web.Duration{Duration: time.Second},
			},
		},
		"fail when network is invalid": {
			wantFail: true,
			config: Config{
				Domains:     []string{"example.com"},
				Servers:     []string{"192.0.2.0"},
				Network:     "gcp",
				RecordTypes: []string{"A"},
				Port:        53,
				Timeout:     web.Duration{Duration: time.Second},
			},
		},
		"fail when record_type is invalid": {
			wantFail: true,
			config: Config{
				Domains:     []string{"example.com"},
				Servers:     []string{"192.0.2.0"},
				Network:     "udp",
				RecordTypes: []string{"B"},
				Port:        53,
				Timeout:     web.Duration{Duration: time.Second},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dq := New()
			dq.Config = test.config

			if test.wantFail {
				assert.False(t, dq.Init())
			} else {
				assert.True(t, dq.Init())
			}
		})
	}
}

func TestDNSQuery_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func() *DNSQuery
	}{
		"success when DNS query successful": {
			wantFail: false,
			prepare:  caseDNSClientOK,
		},
		"success when DNS query returns an error": {
			wantFail: false,
			prepare:  caseDNSClientErr,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dq := test.prepare()

			require.True(t, dq.Init())

			if test.wantFail {
				assert.False(t, dq.Check())
			} else {
				assert.True(t, dq.Check())
			}
		})
	}
}

func TestDNSQuery_Charts(t *testing.T) {
	dq := New()

	dq.Domains = []string{"google.com"}
	dq.Servers = []string{"192.0.2.0", "192.0.2.1"}
	require.True(t, dq.Init())

	assert.NotNil(t, dq.Charts())
	assert.Len(t, *dq.Charts(), len(dnsChartsTmpl)*len(dq.Servers))
}

func TestDNSQuery_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func() *DNSQuery
		wantMetrics map[string]int64
	}{
		"success when DNS query successful": {
			prepare: caseDNSClientOK,
			wantMetrics: map[string]int64{
				"server_192.0.2.0_record_A_query_status_dns_error":     0,
				"server_192.0.2.0_record_A_query_status_network_error": 0,
				"server_192.0.2.0_record_A_query_status_success":       1,
				"server_192.0.2.0_record_A_query_time":                 1000000000,
				"server_192.0.2.1_record_A_query_status_dns_error":     0,
				"server_192.0.2.1_record_A_query_status_network_error": 0,
				"server_192.0.2.1_record_A_query_status_success":       1,
				"server_192.0.2.1_record_A_query_time":                 1000000000,
			},
		},
		"fail when DNS query returns an error": {
			prepare: caseDNSClientErr,
			wantMetrics: map[string]int64{
				"server_192.0.2.0_record_A_query_status_dns_error":     0,
				"server_192.0.2.0_record_A_query_status_network_error": 1,
				"server_192.0.2.0_record_A_query_status_success":       0,
				"server_192.0.2.1_record_A_query_status_dns_error":     0,
				"server_192.0.2.1_record_A_query_status_network_error": 1,
				"server_192.0.2.1_record_A_query_status_success":       0,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dq := test.prepare()

			require.True(t, dq.Init())

			mx := dq.Collect()

			require.Equal(t, test.wantMetrics, mx)
		})
	}
}

func caseDNSClientOK() *DNSQuery {
	dq := New()
	dq.Domains = []string{"example.com"}
	dq.Servers = []string{"192.0.2.0", "192.0.2.1"}
	dq.newDNSClient = func(_ string, _ time.Duration) dnsClient {
		return mockDNSClient{errOnExchange: false}
	}
	return dq
}

func caseDNSClientErr() *DNSQuery {
	dq := New()
	dq.Domains = []string{"example.com"}
	dq.Servers = []string{"192.0.2.0", "192.0.2.1"}
	dq.newDNSClient = func(_ string, _ time.Duration) dnsClient {
		return mockDNSClient{errOnExchange: true}
	}
	return dq
}

type mockDNSClient struct {
	errOnExchange bool
}

func (m mockDNSClient) Exchange(_ *dns.Msg, _ string) (response *dns.Msg, rtt time.Duration, err error) {
	if m.errOnExchange {
		return nil, time.Second, errors.New("mock.Exchange() error")
	}
	return nil, time.Second, nil
}
