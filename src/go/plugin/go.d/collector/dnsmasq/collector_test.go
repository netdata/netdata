// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

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
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset 'address'": {
			wantFail: true,
			config: Config{
				Protocol: "udp",
				Address:  "",
			},
		},
		"fails on unset 'protocol'": {
			wantFail: true,
			config: Config{
				Protocol: "",
				Address:  "127.0.0.1:53",
			},
		},
		"fails on invalid 'protocol'": {
			wantFail: true,
			config: Config{
				Protocol: "http",
				Address:  "127.0.0.1:53",
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
		prepare  func() *Collector
		wantFail bool
	}{
		"success on valid response": {
			prepare: prepareOKDnsmasq,
		},
		"fails on error on cache stats query": {
			wantFail: true,
			prepare:  prepareErrorOnExchangeDnsmasq,
		},
		"fails on response rcode is not success": {
			wantFail: true,
			prepare:  prepareRcodeServerFailureOnExchangeDnsmasq,
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
	require.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *Collector
		wantCollected map[string]int64
	}{
		"success on valid response": {
			prepare: prepareOKDnsmasq,
			wantCollected: map[string]int64{
				//"auth":           5,
				"cachesize":      999,
				"evictions":      5,
				"failed_queries": 9,
				"hits":           100,
				"insertions":     10,
				"misses":         50,
				"queries":        17,
			},
		},
		"fails on error on cache stats query": {
			prepare: prepareErrorOnExchangeDnsmasq,
		},
		"fails on response rcode is not success": {
			prepare: prepareRcodeServerFailureOnExchangeDnsmasq,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()
			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
			if len(test.wantCollected) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareOKDnsmasq() *Collector {
	collr := New()
	collr.newDNSClient = func(network string, timeout time.Duration) dnsClient {
		return &mockDNSClient{}
	}
	return collr
}

func prepareErrorOnExchangeDnsmasq() *Collector {
	collr := New()
	collr.newDNSClient = func(network string, timeout time.Duration) dnsClient {
		return &mockDNSClient{
			errOnExchange: true,
		}
	}
	return collr
}

func prepareRcodeServerFailureOnExchangeDnsmasq() *Collector {
	collr := New()
	collr.newDNSClient = func(network string, timeout time.Duration) dnsClient {
		return &mockDNSClient{
			rcodeServerFailureOnExchange: true,
		}
	}
	return collr
}

type mockDNSClient struct {
	errOnExchange                bool
	rcodeServerFailureOnExchange bool
}

func (m mockDNSClient) Exchange(msg *dns.Msg, _ string) (*dns.Msg, time.Duration, error) {
	if m.errOnExchange {
		return nil, 0, errors.New("'Exchange' error")
	}
	if m.rcodeServerFailureOnExchange {
		resp := &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: dns.RcodeServerFailure}}
		return resp, 0, nil
	}

	var answers []dns.RR
	for _, q := range msg.Question {
		a, err := prepareDNSAnswer(q)
		if err != nil {
			return nil, 0, err
		}
		answers = append(answers, a)
	}

	resp := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Rcode: dns.RcodeSuccess,
		},
		Answer: answers,
	}
	return resp, 0, nil
}

func prepareDNSAnswer(q dns.Question) (dns.RR, error) {
	if want, got := dns.TypeToString[dns.TypeTXT], dns.TypeToString[q.Qtype]; want != got {
		return nil, fmt.Errorf("unexpected Qtype, want=%s, got=%s", want, got)
	}
	if want, got := dns.ClassToString[dns.ClassCHAOS], dns.ClassToString[q.Qclass]; want != got {
		return nil, fmt.Errorf("unexpected Qclass, want=%s, got=%s", want, got)
	}

	var txt []string
	switch q.Name {
	case "cachesize.bind.":
		txt = []string{"999"}
	case "insertions.bind.":
		txt = []string{"10"}
	case "evictions.bind.":
		txt = []string{"5"}
	case "hits.bind.":
		txt = []string{"100"}
	case "misses.bind.":
		txt = []string{"50"}
	case "auth.bind.":
		txt = []string{"5"}
	case "servers.bind.":
		txt = []string{"10.0.0.1#53 10 5", "1.1.1.1#53 4 3", "1.0.0.1#53 3 1"}
	default:
		return nil, fmt.Errorf("unexpected question Name: %s", q.Name)
	}

	rr := &dns.TXT{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassCHAOS,
		},
		Txt: txt,
	}
	return rr, nil
}
