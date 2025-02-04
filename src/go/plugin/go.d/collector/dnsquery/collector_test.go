// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

	"github.com/miekg/dns"
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

func TestCollector_Init(t *testing.T) {
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
				Timeout:     confopt.Duration(time.Second),
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
				Timeout:    confopt.Duration(time.Second),
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
				Timeout:     confopt.Duration(time.Second),
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
				Timeout:     confopt.Duration(time.Second),
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
				Timeout:     confopt.Duration(time.Second),
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
		prepare  func() *Collector
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
			collr := test.prepare()

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

	collr.Domains = []string{"google.com"}
	collr.Servers = []string{"192.0.2.0", "192.0.2.1"}
	require.NoError(t, collr.Init(context.Background()))

	assert.NotNil(t, collr.Charts())
	assert.Len(t, *collr.Charts(), len(dnsChartsTmpl)*len(collr.Servers))
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func() *Collector
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
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)
		})
	}
}

func caseDNSClientOK() *Collector {
	collr := New()
	collr.Domains = []string{"example.com"}
	collr.Servers = []string{"192.0.2.0", "192.0.2.1"}
	collr.newDNSClient = func(_ string, _ time.Duration) dnsClient {
		return mockDNSClient{errOnExchange: false}
	}
	return collr
}

func caseDNSClientErr() *Collector {
	collr := New()
	collr.Domains = []string{"example.com"}
	collr.Servers = []string{"192.0.2.0", "192.0.2.1"}
	collr.newDNSClient = func(_ string, _ time.Duration) dnsClient {
		return mockDNSClient{errOnExchange: true}
	}
	return collr
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
