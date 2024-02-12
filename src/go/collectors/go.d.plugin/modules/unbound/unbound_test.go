// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/netdata/go.d.plugin/pkg/socket"
	"github.com/netdata/go.d.plugin/pkg/tlscfg"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	commonStatsData, _          = os.ReadFile("testdata/stats/common.txt")
	extStatsData, _             = os.ReadFile("testdata/stats/extended.txt")
	lifeCycleCumulativeData1, _ = os.ReadFile("testdata/stats/lifecycle/cumulative/extended1.txt")
	lifeCycleCumulativeData2, _ = os.ReadFile("testdata/stats/lifecycle/cumulative/extended2.txt")
	lifeCycleCumulativeData3, _ = os.ReadFile("testdata/stats/lifecycle/cumulative/extended3.txt")
	lifeCycleResetData1, _      = os.ReadFile("testdata/stats/lifecycle/reset/extended1.txt")
	lifeCycleResetData2, _      = os.ReadFile("testdata/stats/lifecycle/reset/extended2.txt")
	lifeCycleResetData3, _      = os.ReadFile("testdata/stats/lifecycle/reset/extended3.txt")
)

func Test_readTestData(t *testing.T) {
	assert.NotNil(t, commonStatsData)
	assert.NotNil(t, extStatsData)
	assert.NotNil(t, lifeCycleCumulativeData1)
	assert.NotNil(t, lifeCycleCumulativeData2)
	assert.NotNil(t, lifeCycleCumulativeData3)
	assert.NotNil(t, lifeCycleResetData1)
	assert.NotNil(t, lifeCycleResetData2)
	assert.NotNil(t, lifeCycleResetData3)
}

func nonTLSUnbound() *Unbound {
	unbound := New()
	unbound.ConfPath = ""
	unbound.UseTLS = false
	return unbound
}

func TestNew(t *testing.T) {
	assert.Implements(t, (*module.Module)(nil), New())
}

func TestUnbound_Init(t *testing.T) {
	unbound := nonTLSUnbound()

	assert.True(t, unbound.Init())
}

func TestUnbound_Init_SetEverythingFromUnboundConf(t *testing.T) {
	unbound := New()
	unbound.ConfPath = "testdata/unbound.conf"
	expectedConfig := Config{
		Address:    "10.0.0.1:8954",
		ConfPath:   unbound.ConfPath,
		Timeout:    unbound.Timeout,
		Cumulative: true,
		UseTLS:     false,
		TLSConfig: tlscfg.TLSConfig{
			TLSCert:            "/etc/unbound/unbound_control_other.pem",
			TLSKey:             "/etc/unbound/unbound_control_other.key",
			InsecureSkipVerify: unbound.TLSConfig.InsecureSkipVerify,
		},
	}

	assert.True(t, unbound.Init())
	assert.Equal(t, expectedConfig, unbound.Config)
}

func TestUnbound_Init_DisabledInUnboundConf(t *testing.T) {
	unbound := nonTLSUnbound()
	unbound.ConfPath = "testdata/unbound_disabled.conf"

	assert.False(t, unbound.Init())
}

func TestUnbound_Init_HandleEmptyConfig(t *testing.T) {
	unbound := nonTLSUnbound()
	unbound.ConfPath = "testdata/unbound_empty.conf"

	assert.True(t, unbound.Init())
}

func TestUnbound_Init_HandleNonExistentConfig(t *testing.T) {
	unbound := nonTLSUnbound()
	unbound.ConfPath = "testdata/unbound_non_existent.conf"

	assert.True(t, unbound.Init())
}

func TestUnbound_Check(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())
	unbound.client = mockUnboundClient{data: commonStatsData, err: false}

	assert.True(t, unbound.Check())
}

func TestUnbound_Check_ErrorDuringScrapingUnbound(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())
	unbound.client = mockUnboundClient{err: true}

	assert.False(t, unbound.Check())
}

func TestUnbound_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestUnbound_Charts(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())

	assert.NotNil(t, unbound.Charts())
}

func TestUnbound_Collect(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())
	unbound.client = mockUnboundClient{data: commonStatsData, err: false}

	collected := unbound.Collect()
	assert.Equal(t, expectedCommon, collected)
	testCharts(t, unbound, collected)
}

func TestUnbound_Collect_ExtendedStats(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())
	unbound.client = mockUnboundClient{data: extStatsData, err: false}

	collected := unbound.Collect()
	assert.Equal(t, expectedExtended, collected)
	testCharts(t, unbound, collected)
}

func TestUnbound_Collect_LifeCycleCumulativeExtendedStats(t *testing.T) {
	tests := []struct {
		input    []byte
		expected map[string]int64
	}{
		{input: lifeCycleCumulativeData1, expected: expectedCumulative1},
		{input: lifeCycleCumulativeData2, expected: expectedCumulative2},
		{input: lifeCycleCumulativeData3, expected: expectedCumulative3},
	}

	unbound := nonTLSUnbound()
	unbound.Cumulative = true
	require.True(t, unbound.Init())
	ubClient := &mockUnboundClient{err: false}
	unbound.client = ubClient

	var collected map[string]int64
	for i, test := range tests {
		t.Run(fmt.Sprintf("run %d", i+1), func(t *testing.T) {
			ubClient.data = test.input
			collected = unbound.Collect()
			assert.Equal(t, test.expected, collected)
		})
	}

	testCharts(t, unbound, collected)
}

func TestUnbound_Collect_LifeCycleResetExtendedStats(t *testing.T) {
	tests := []struct {
		input    []byte
		expected map[string]int64
	}{
		{input: lifeCycleResetData1, expected: expectedReset1},
		{input: lifeCycleResetData2, expected: expectedReset2},
		{input: lifeCycleResetData3, expected: expectedReset3},
	}

	unbound := nonTLSUnbound()
	unbound.Cumulative = false
	require.True(t, unbound.Init())
	ubClient := &mockUnboundClient{err: false}
	unbound.client = ubClient

	var collected map[string]int64
	for i, test := range tests {
		t.Run(fmt.Sprintf("run %d", i+1), func(t *testing.T) {
			ubClient.data = test.input
			collected = unbound.Collect()
			assert.Equal(t, test.expected, collected)
		})
	}

	testCharts(t, unbound, collected)
}

func TestUnbound_Collect_EmptyResponse(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())
	unbound.client = mockUnboundClient{data: []byte{}, err: false}

	assert.Nil(t, unbound.Collect())
}

func TestUnbound_Collect_ErrorResponse(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())
	unbound.client = mockUnboundClient{data: []byte("error unknown command 'unknown'"), err: false}

	assert.Nil(t, unbound.Collect())
}

func TestUnbound_Collect_ErrorOnSend(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())
	unbound.client = mockUnboundClient{err: true}

	assert.Nil(t, unbound.Collect())
}

func TestUnbound_Collect_ErrorOnParseBadSyntax(t *testing.T) {
	unbound := nonTLSUnbound()
	require.True(t, unbound.Init())
	data := strings.Repeat("zk_avg_latency	0\nzk_min_latency	0\nzk_mix_latency	0\n", 10)
	unbound.client = mockUnboundClient{data: []byte(data), err: false}

	assert.Nil(t, unbound.Collect())
}

type mockUnboundClient struct {
	data []byte
	err  bool
}

func (m mockUnboundClient) Connect() error {
	return nil
}

func (m mockUnboundClient) Disconnect() error {
	return nil
}

func (m mockUnboundClient) Command(_ string, process socket.Processor) error {
	if m.err {
		return errors.New("mock send error")
	}
	s := bufio.NewScanner(bytes.NewReader(m.data))
	for s.Scan() {
		process(s.Bytes())
	}
	return nil
}

func testCharts(t *testing.T, unbound *Unbound, collected map[string]int64) {
	t.Helper()
	ensureChartsCreatedForEveryThread(t, unbound)
	ensureExtendedChartsCreated(t, unbound)
	ensureCollectedHasAllChartsDimsVarsIDs(t, unbound, collected)
}

func ensureChartsCreatedForEveryThread(t *testing.T, u *Unbound) {
	for thread := range u.cache.threads {
		for _, chart := range *threadCharts(thread, u.Cumulative) {
			assert.Truef(t, u.Charts().Has(chart.ID), "chart '%s' is not created for '%s' thread", chart.ID, thread)
		}
	}
}

func ensureExtendedChartsCreated(t *testing.T, u *Unbound) {
	if len(u.cache.answerRCode) == 0 {
		return
	}
	for _, chart := range *extendedCharts(u.Cumulative) {
		assert.Truef(t, u.Charts().Has(chart.ID), "chart '%s' is not added", chart.ID)
	}

	if chart := u.Charts().Get(queryTypeChart.ID); chart != nil {
		for typ := range u.cache.queryType {
			dimID := "num.query.type." + typ
			assert.Truef(t, chart.HasDim(dimID), "chart '%s' has no dim for '%s' type, expected '%s'", chart.ID, typ, dimID)
		}
	}
	if chart := u.Charts().Get(queryClassChart.ID); chart != nil {
		for class := range u.cache.queryClass {
			dimID := "num.query.class." + class
			assert.Truef(t, chart.HasDim(dimID), "chart '%s' has no dim for '%s' class, expected '%s'", chart.ID, class, dimID)
		}
	}
	if chart := u.Charts().Get(queryOpCodeChart.ID); chart != nil {
		for opcode := range u.cache.queryOpCode {
			dimID := "num.query.opcode." + opcode
			assert.Truef(t, chart.HasDim(dimID), "chart '%s' has no dim for '%s' opcode, expected '%s'", chart.ID, opcode, dimID)
		}
	}
	if chart := u.Charts().Get(answerRCodeChart.ID); chart != nil {
		for rcode := range u.cache.answerRCode {
			dimID := "num.answer.rcode." + rcode
			assert.Truef(t, chart.HasDim(dimID), "chart '%s' has no dim for '%s' rcode, expected '%s'", chart.ID, rcode, dimID)
		}
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, u *Unbound, collected map[string]int64) {
	for _, chart := range *u.Charts() {
		for _, dim := range chart.Dims {
			if dim.ID == "mem.mod.ipsecmod" {
				continue
			}
			_, ok := collected[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

var (
	expectedCommon = map[string]int64{
		"thread0.num.cachehits":              21,
		"thread0.num.cachemiss":              7,
		"thread0.num.dnscrypt.cert":          0,
		"thread0.num.dnscrypt.cleartext":     0,
		"thread0.num.dnscrypt.crypted":       0,
		"thread0.num.dnscrypt.malformed":     0,
		"thread0.num.expired":                0,
		"thread0.num.prefetch":               0,
		"thread0.num.queries":                28,
		"thread0.num.queries_ip_ratelimited": 0,
		"thread0.num.recursivereplies":       7,
		"thread0.num.zero_ttl":               0,
		"thread0.recursion.time.avg":         1255,
		"thread0.recursion.time.median":      480,
		"thread0.requestlist.avg":            857,
		"thread0.requestlist.current.all":    0,
		"thread0.requestlist.current.user":   0,
		"thread0.requestlist.exceeded":       0,
		"thread0.requestlist.max":            6,
		"thread0.requestlist.overwritten":    0,
		"thread0.tcpusage":                   0,
		"thread1.num.cachehits":              13,
		"thread1.num.cachemiss":              3,
		"thread1.num.dnscrypt.cert":          0,
		"thread1.num.dnscrypt.cleartext":     0,
		"thread1.num.dnscrypt.crypted":       0,
		"thread1.num.dnscrypt.malformed":     0,
		"thread1.num.prefetch":               0,
		"thread1.num.expired":                0,
		"thread1.num.queries":                16,
		"thread1.num.queries_ip_ratelimited": 0,
		"thread1.num.recursivereplies":       3,
		"thread1.num.zero_ttl":               0,
		"thread1.recursion.time.avg":         93,
		"thread1.recursion.time.median":      0,
		"thread1.requestlist.avg":            0,
		"thread1.requestlist.current.all":    0,
		"thread1.requestlist.current.user":   0,
		"thread1.requestlist.exceeded":       0,
		"thread1.requestlist.max":            0,
		"thread1.requestlist.overwritten":    0,
		"thread1.tcpusage":                   0,
		"time.elapsed":                       88,
		"time.now":                           1574094836,
		"time.up":                            88,
		"total.num.cachehits":                34,
		"total.num.cachemiss":                10,
		"total.num.dnscrypt.cert":            0,
		"total.num.dnscrypt.cleartext":       0,
		"total.num.dnscrypt.crypted":         0,
		"total.num.dnscrypt.malformed":       0,
		"total.num.prefetch":                 0,
		"total.num.expired":                  0,
		"total.num.queries":                  44,
		"total.num.queries_ip_ratelimited":   0,
		"total.num.recursivereplies":         10,
		"total.num.zero_ttl":                 0,
		"total.recursion.time.avg":           907,
		"total.recursion.time.median":        240,
		"total.requestlist.avg":              600,
		"total.requestlist.current.all":      0,
		"total.requestlist.current.user":     0,
		"total.requestlist.exceeded":         0,
		"total.requestlist.max":              6,
		"total.requestlist.overwritten":      0,
		"total.tcpusage":                     0,
	}

	expectedExtended = map[string]int64{
		"dnscrypt_nonce.cache.count":                 0,
		"dnscrypt_shared_secret.cache.count":         0,
		"infra.cache.count":                          205,
		"key.cache.count":                            9,
		"mem.cache.dnscrypt_nonce":                   0,
		"mem.cache.dnscrypt_shared_secret":           0,
		"mem.cache.message":                          90357,
		"mem.cache.rrset":                            178642,
		"mem.mod.iterator":                           16588,
		"mem.mod.respip":                             0,
		"mem.mod.subnet":                             74504,
		"mem.mod.validator":                          81059,
		"mem.streamwait":                             0,
		"msg.cache.count":                            81,
		"num.answer.bogus":                           0,
		"num.answer.rcode.FORMERR":                   0,
		"num.answer.rcode.NOERROR":                   40,
		"num.answer.rcode.NOTIMPL":                   0,
		"num.answer.rcode.NXDOMAIN":                  4,
		"num.answer.rcode.REFUSED":                   0,
		"num.answer.rcode.SERVFAIL":                  0,
		"num.answer.secure":                          0,
		"num.query.aggressive.NOERROR":               2,
		"num.query.aggressive.NXDOMAIN":              0,
		"num.query.authzone.down":                    0,
		"num.query.authzone.up":                      0,
		"num.query.class.IN":                         44,
		"num.query.dnscrypt.replay":                  0,
		"num.query.dnscrypt.shared_secret.cachemiss": 0,
		"num.query.edns.DO":                          0,
		"num.query.edns.present":                     0,
		"num.query.flags.AA":                         0,
		"num.query.flags.AD":                         0,
		"num.query.flags.CD":                         0,
		"num.query.flags.QR":                         0,
		"num.query.flags.RA":                         0,
		"num.query.flags.RD":                         44,
		"num.query.flags.TC":                         0,
		"num.query.flags.Z":                          0,
		"num.query.ipv6":                             39,
		"num.query.opcode.QUERY":                     44,
		"num.query.ratelimited":                      0,
		"num.query.subnet":                           0,
		"num.query.subnet_cache":                     0,
		"num.query.tcp":                              0,
		"num.query.tcpout":                           1,
		"num.query.tls":                              0,
		"num.query.tls.resume":                       0,
		"num.query.type.A":                           13,
		"num.query.type.AAAA":                        13,
		"num.query.type.MX":                          13,
		"num.query.type.PTR":                         5,
		"num.rrset.bogus":                            0,
		"rrset.cache.count":                          314,
		"thread0.num.cachehits":                      21,
		"thread0.num.cachemiss":                      7,
		"thread0.num.dnscrypt.cert":                  0,
		"thread0.num.dnscrypt.cleartext":             0,
		"thread0.num.dnscrypt.crypted":               0,
		"thread0.num.dnscrypt.malformed":             0,
		"thread0.num.expired":                        0,
		"thread0.num.prefetch":                       0,
		"thread0.num.queries":                        28,
		"thread0.num.queries_ip_ratelimited":         0,
		"thread0.num.recursivereplies":               7,
		"thread0.num.zero_ttl":                       0,
		"thread0.recursion.time.avg":                 1255,
		"thread0.recursion.time.median":              480,
		"thread0.requestlist.avg":                    857,
		"thread0.requestlist.current.all":            0,
		"thread0.requestlist.current.user":           0,
		"thread0.requestlist.exceeded":               0,
		"thread0.requestlist.max":                    6,
		"thread0.requestlist.overwritten":            0,
		"thread0.tcpusage":                           0,
		"thread1.num.cachehits":                      13,
		"thread1.num.cachemiss":                      3,
		"thread1.num.dnscrypt.cert":                  0,
		"thread1.num.dnscrypt.cleartext":             0,
		"thread1.num.dnscrypt.crypted":               0,
		"thread1.num.dnscrypt.malformed":             0,
		"thread1.num.prefetch":                       0,
		"thread1.num.expired":                        0,
		"thread1.num.queries":                        16,
		"thread1.num.queries_ip_ratelimited":         0,
		"thread1.num.recursivereplies":               3,
		"thread1.num.zero_ttl":                       0,
		"thread1.recursion.time.avg":                 93,
		"thread1.recursion.time.median":              0,
		"thread1.requestlist.avg":                    0,
		"thread1.requestlist.current.all":            0,
		"thread1.requestlist.current.user":           0,
		"thread1.requestlist.exceeded":               0,
		"thread1.requestlist.max":                    0,
		"thread1.requestlist.overwritten":            0,
		"thread1.tcpusage":                           0,
		"time.elapsed":                               88,
		"time.now":                                   1574094836,
		"time.up":                                    88,
		"total.num.cachehits":                        34,
		"total.num.cachemiss":                        10,
		"total.num.dnscrypt.cert":                    0,
		"total.num.dnscrypt.cleartext":               0,
		"total.num.dnscrypt.crypted":                 0,
		"total.num.dnscrypt.malformed":               0,
		"total.num.prefetch":                         0,
		"total.num.expired":                          0,
		"total.num.queries":                          44,
		"total.num.queries_ip_ratelimited":           0,
		"total.num.recursivereplies":                 10,
		"total.num.zero_ttl":                         0,
		"total.recursion.time.avg":                   907,
		"total.recursion.time.median":                240,
		"total.requestlist.avg":                      600,
		"total.requestlist.current.all":              0,
		"total.requestlist.current.user":             0,
		"total.requestlist.exceeded":                 0,
		"total.requestlist.max":                      6,
		"total.requestlist.overwritten":              0,
		"total.tcpusage":                             0,
		"unwanted.queries":                           0,
		"unwanted.replies":                           0,
	}
)

var (
	expectedCumulative1 = map[string]int64{
		"dnscrypt_nonce.cache.count":                 0,
		"dnscrypt_shared_secret.cache.count":         0,
		"infra.cache.count":                          192,
		"key.cache.count":                            11,
		"mem.cache.dnscrypt_nonce":                   0,
		"mem.cache.dnscrypt_shared_secret":           0,
		"mem.cache.message":                          93392,
		"mem.cache.rrset":                            175745,
		"mem.mod.iterator":                           16588,
		"mem.mod.respip":                             0,
		"mem.mod.subnet":                             74504,
		"mem.mod.validator":                          81479,
		"mem.streamwait":                             0,
		"msg.cache.count":                            94,
		"num.answer.bogus":                           0,
		"num.answer.rcode.FORMERR":                   0,
		"num.answer.rcode.NOERROR":                   184,
		"num.answer.rcode.NOTIMPL":                   0,
		"num.answer.rcode.NXDOMAIN":                  16,
		"num.answer.rcode.REFUSED":                   0,
		"num.answer.rcode.SERVFAIL":                  0,
		"num.answer.secure":                          0,
		"num.query.aggressive.NOERROR":               1,
		"num.query.aggressive.NXDOMAIN":              0,
		"num.query.authzone.down":                    0,
		"num.query.authzone.up":                      0,
		"num.query.class.IN":                         200,
		"num.query.dnscrypt.replay":                  0,
		"num.query.dnscrypt.shared_secret.cachemiss": 0,
		"num.query.edns.DO":                          0,
		"num.query.edns.present":                     0,
		"num.query.flags.AA":                         0,
		"num.query.flags.AD":                         0,
		"num.query.flags.CD":                         0,
		"num.query.flags.QR":                         0,
		"num.query.flags.RA":                         0,
		"num.query.flags.RD":                         200,
		"num.query.flags.TC":                         0,
		"num.query.flags.Z":                          0,
		"num.query.ipv6":                             0,
		"num.query.opcode.QUERY":                     200,
		"num.query.ratelimited":                      0,
		"num.query.subnet":                           0,
		"num.query.subnet_cache":                     0,
		"num.query.tcp":                              0,
		"num.query.tcpout":                           0,
		"num.query.tls":                              0,
		"num.query.tls.resume":                       0,
		"num.query.type.A":                           60,
		"num.query.type.AAAA":                        60,
		"num.query.type.MX":                          60,
		"num.query.type.PTR":                         20,
		"num.rrset.bogus":                            0,
		"rrset.cache.count":                          304,
		"thread0.num.cachehits":                      80,
		"thread0.num.cachemiss":                      10,
		"thread0.num.dnscrypt.cert":                  0,
		"thread0.num.dnscrypt.cleartext":             0,
		"thread0.num.dnscrypt.crypted":               0,
		"thread0.num.dnscrypt.malformed":             0,
		"thread0.num.expired":                        0,
		"thread0.num.prefetch":                       0,
		"thread0.num.queries":                        90,
		"thread0.num.queries_ip_ratelimited":         0,
		"thread0.num.recursivereplies":               10,
		"thread0.num.zero_ttl":                       0,
		"thread0.recursion.time.avg":                 222,
		"thread0.recursion.time.median":              337,
		"thread0.requestlist.avg":                    100,
		"thread0.requestlist.current.all":            0,
		"thread0.requestlist.current.user":           0,
		"thread0.requestlist.exceeded":               0,
		"thread0.requestlist.max":                    1,
		"thread0.requestlist.overwritten":            0,
		"thread0.tcpusage":                           0,
		"thread1.num.cachehits":                      101,
		"thread1.num.cachemiss":                      9,
		"thread1.num.dnscrypt.cert":                  0,
		"thread1.num.dnscrypt.cleartext":             0,
		"thread1.num.dnscrypt.crypted":               0,
		"thread1.num.dnscrypt.malformed":             0,
		"thread1.num.expired":                        0,
		"thread1.num.prefetch":                       0,
		"thread1.num.queries":                        110,
		"thread1.num.queries_ip_ratelimited":         0,
		"thread1.num.recursivereplies":               9,
		"thread1.num.zero_ttl":                       0,
		"thread1.recursion.time.avg":                 844,
		"thread1.recursion.time.median":              360,
		"thread1.requestlist.avg":                    222,
		"thread1.requestlist.current.all":            0,
		"thread1.requestlist.current.user":           0,
		"thread1.requestlist.exceeded":               0,
		"thread1.requestlist.max":                    1,
		"thread1.requestlist.overwritten":            0,
		"thread1.tcpusage":                           0,
		"time.elapsed":                               122,
		"time.now":                                   1574103378,
		"time.up":                                    122,
		"total.num.cachehits":                        181,
		"total.num.cachemiss":                        19,
		"total.num.dnscrypt.cert":                    0,
		"total.num.dnscrypt.cleartext":               0,
		"total.num.dnscrypt.crypted":                 0,
		"total.num.dnscrypt.malformed":               0,
		"total.num.expired":                          0,
		"total.num.prefetch":                         0,
		"total.num.queries":                          200,
		"total.num.queries_ip_ratelimited":           0,
		"total.num.recursivereplies":                 19,
		"total.num.zero_ttl":                         0,
		"total.recursion.time.avg":                   516,
		"total.recursion.time.median":                348,
		"total.requestlist.avg":                      157,
		"total.requestlist.current.all":              0,
		"total.requestlist.current.user":             0,
		"total.requestlist.exceeded":                 0,
		"total.requestlist.max":                      1,
		"total.requestlist.overwritten":              0,
		"total.tcpusage":                             0,
		"unwanted.queries":                           0,
		"unwanted.replies":                           0,
	}

	expectedCumulative2 = map[string]int64{
		"dnscrypt_nonce.cache.count":                 0,
		"dnscrypt_shared_secret.cache.count":         0,
		"infra.cache.count":                          192,
		"key.cache.count":                            11,
		"mem.cache.dnscrypt_nonce":                   0,
		"mem.cache.dnscrypt_shared_secret":           0,
		"mem.cache.message":                          93392,
		"mem.cache.rrset":                            175745,
		"mem.mod.iterator":                           16588,
		"mem.mod.respip":                             0,
		"mem.mod.subnet":                             74504,
		"mem.mod.validator":                          81479,
		"mem.streamwait":                             0,
		"msg.cache.count":                            94,
		"num.answer.bogus":                           0,
		"num.answer.rcode.FORMERR":                   0,
		"num.answer.rcode.NOERROR":                   274,
		"num.answer.rcode.NOTIMPL":                   0,
		"num.answer.rcode.NXDOMAIN":                  16,
		"num.answer.rcode.REFUSED":                   0,
		"num.answer.rcode.SERVFAIL":                  0,
		"num.answer.secure":                          0,
		"num.query.aggressive.NOERROR":               1,
		"num.query.aggressive.NXDOMAIN":              0,
		"num.query.authzone.down":                    0,
		"num.query.authzone.up":                      0,
		"num.query.class.IN":                         290,
		"num.query.dnscrypt.replay":                  0,
		"num.query.dnscrypt.shared_secret.cachemiss": 0,
		"num.query.edns.DO":                          0,
		"num.query.edns.present":                     0,
		"num.query.flags.AA":                         0,
		"num.query.flags.AD":                         0,
		"num.query.flags.CD":                         0,
		"num.query.flags.QR":                         0,
		"num.query.flags.RA":                         0,
		"num.query.flags.RD":                         290,
		"num.query.flags.TC":                         0,
		"num.query.flags.Z":                          0,
		"num.query.ipv6":                             0,
		"num.query.opcode.QUERY":                     290,
		"num.query.ratelimited":                      0,
		"num.query.subnet":                           0,
		"num.query.subnet_cache":                     0,
		"num.query.tcp":                              0,
		"num.query.tcpout":                           0,
		"num.query.tls":                              0,
		"num.query.tls.resume":                       0,
		"num.query.type.A":                           90,
		"num.query.type.AAAA":                        90,
		"num.query.type.MX":                          90,
		"num.query.type.PTR":                         20,
		"num.rrset.bogus":                            0,
		"rrset.cache.count":                          304,
		"thread0.num.cachehits":                      123,
		"thread0.num.cachemiss":                      10,
		"thread0.num.dnscrypt.cert":                  0,
		"thread0.num.dnscrypt.cleartext":             0,
		"thread0.num.dnscrypt.crypted":               0,
		"thread0.num.dnscrypt.malformed":             0,
		"thread0.num.expired":                        0,
		"thread0.num.prefetch":                       0,
		"thread0.num.queries":                        133,
		"thread0.num.queries_ip_ratelimited":         0,
		"thread0.num.recursivereplies":               10,
		"thread0.num.zero_ttl":                       0,
		"thread0.recursion.time.avg":                 0,
		"thread0.recursion.time.median":              0,
		"thread0.requestlist.avg":                    0,
		"thread0.requestlist.current.all":            0,
		"thread0.requestlist.current.user":           0,
		"thread0.requestlist.exceeded":               0,
		"thread0.requestlist.max":                    1,
		"thread0.requestlist.overwritten":            0,
		"thread0.tcpusage":                           0,
		"thread1.num.cachehits":                      148,
		"thread1.num.cachemiss":                      9,
		"thread1.num.dnscrypt.cert":                  0,
		"thread1.num.dnscrypt.cleartext":             0,
		"thread1.num.dnscrypt.crypted":               0,
		"thread1.num.dnscrypt.malformed":             0,
		"thread1.num.prefetch":                       0,
		"thread1.num.expired":                        0,
		"thread1.num.queries":                        157,
		"thread1.num.queries_ip_ratelimited":         0,
		"thread1.num.recursivereplies":               9,
		"thread1.num.zero_ttl":                       0,
		"thread1.recursion.time.avg":                 0,
		"thread1.recursion.time.median":              0,
		"thread1.requestlist.avg":                    0,
		"thread1.requestlist.current.all":            0,
		"thread1.requestlist.current.user":           0,
		"thread1.requestlist.exceeded":               0,
		"thread1.requestlist.max":                    1,
		"thread1.requestlist.overwritten":            0,
		"thread1.tcpusage":                           0,
		"time.elapsed":                               82,
		"time.now":                                   1574103461,
		"time.up":                                    205,
		"total.num.cachehits":                        271,
		"total.num.cachemiss":                        19,
		"total.num.dnscrypt.cert":                    0,
		"total.num.dnscrypt.cleartext":               0,
		"total.num.dnscrypt.crypted":                 0,
		"total.num.dnscrypt.malformed":               0,
		"total.num.prefetch":                         0,
		"total.num.expired":                          0,
		"total.num.queries":                          290,
		"total.num.queries_ip_ratelimited":           0,
		"total.num.recursivereplies":                 19,
		"total.num.zero_ttl":                         0,
		"total.recursion.time.avg":                   0,
		"total.recursion.time.median":                0,
		"total.requestlist.avg":                      0,
		"total.requestlist.current.all":              0,
		"total.requestlist.current.user":             0,
		"total.requestlist.exceeded":                 0,
		"total.requestlist.max":                      1,
		"total.requestlist.overwritten":              0,
		"total.tcpusage":                             0,
		"unwanted.queries":                           0,
		"unwanted.replies":                           0,
	}

	expectedCumulative3 = map[string]int64{
		"dnscrypt_nonce.cache.count":                 0,
		"dnscrypt_shared_secret.cache.count":         0,
		"infra.cache.count":                          232,
		"key.cache.count":                            14,
		"mem.cache.dnscrypt_nonce":                   0,
		"mem.cache.dnscrypt_shared_secret":           0,
		"mem.cache.message":                          101198,
		"mem.cache.rrset":                            208839,
		"mem.mod.iterator":                           16588,
		"mem.mod.respip":                             0,
		"mem.mod.subnet":                             74504,
		"mem.mod.validator":                          85725,
		"mem.streamwait":                             0,
		"msg.cache.count":                            119,
		"num.answer.bogus":                           0,
		"num.answer.rcode.FORMERR":                   0,
		"num.answer.rcode.NOERROR":                   334,
		"num.answer.rcode.NOTIMPL":                   0,
		"num.answer.rcode.NXDOMAIN":                  16,
		"num.answer.rcode.REFUSED":                   0,
		"num.answer.rcode.SERVFAIL":                  10,
		"num.answer.rcode.nodata":                    20,
		"num.answer.secure":                          0,
		"num.query.aggressive.NOERROR":               1,
		"num.query.aggressive.NXDOMAIN":              0,
		"num.query.authzone.down":                    0,
		"num.query.authzone.up":                      0,
		"num.query.class.IN":                         360,
		"num.query.dnscrypt.replay":                  0,
		"num.query.dnscrypt.shared_secret.cachemiss": 0,
		"num.query.edns.DO":                          0,
		"num.query.edns.present":                     0,
		"num.query.flags.AA":                         0,
		"num.query.flags.AD":                         0,
		"num.query.flags.CD":                         0,
		"num.query.flags.QR":                         0,
		"num.query.flags.RA":                         0,
		"num.query.flags.RD":                         360,
		"num.query.flags.TC":                         0,
		"num.query.flags.Z":                          0,
		"num.query.ipv6":                             0,
		"num.query.opcode.QUERY":                     360,
		"num.query.ratelimited":                      0,
		"num.query.subnet":                           0,
		"num.query.subnet_cache":                     0,
		"num.query.tcp":                              0,
		"num.query.tcpout":                           0,
		"num.query.tls":                              0,
		"num.query.tls.resume":                       0,
		"num.query.type.A":                           120,
		"num.query.type.AAAA":                        110,
		"num.query.type.MX":                          110,
		"num.query.type.PTR":                         20,
		"num.rrset.bogus":                            0,
		"rrset.cache.count":                          401,
		"thread0.num.cachehits":                      150,
		"thread0.num.cachemiss":                      15,
		"thread0.num.dnscrypt.cert":                  0,
		"thread0.num.dnscrypt.cleartext":             0,
		"thread0.num.dnscrypt.crypted":               0,
		"thread0.num.dnscrypt.malformed":             0,
		"thread0.num.expired":                        0,
		"thread0.num.prefetch":                       0,
		"thread0.num.queries":                        165,
		"thread0.num.queries_ip_ratelimited":         0,
		"thread0.num.recursivereplies":               15,
		"thread0.num.zero_ttl":                       0,
		"thread0.recursion.time.avg":                 261,
		"thread0.recursion.time.median":              318,
		"thread0.requestlist.avg":                    66,
		"thread0.requestlist.current.all":            0,
		"thread0.requestlist.current.user":           0,
		"thread0.requestlist.exceeded":               0,
		"thread0.requestlist.max":                    1,
		"thread0.requestlist.overwritten":            0,
		"thread0.tcpusage":                           0,
		"thread1.num.cachehits":                      184,
		"thread1.num.cachemiss":                      11,
		"thread1.num.dnscrypt.cert":                  0,
		"thread1.num.dnscrypt.cleartext":             0,
		"thread1.num.dnscrypt.crypted":               0,
		"thread1.num.dnscrypt.malformed":             0,
		"thread1.num.prefetch":                       0,
		"thread1.num.expired":                        0,
		"thread1.num.queries":                        195,
		"thread1.num.queries_ip_ratelimited":         0,
		"thread1.num.recursivereplies":               11,
		"thread1.num.zero_ttl":                       0,
		"thread1.recursion.time.avg":                 709,
		"thread1.recursion.time.median":              294,
		"thread1.requestlist.avg":                    363,
		"thread1.requestlist.current.all":            0,
		"thread1.requestlist.current.user":           0,
		"thread1.requestlist.exceeded":               0,
		"thread1.requestlist.max":                    2,
		"thread1.requestlist.overwritten":            0,
		"thread1.tcpusage":                           0,
		"time.elapsed":                               82,
		"time.now":                                   1574103543,
		"time.up":                                    288,
		"total.num.cachehits":                        334,
		"total.num.cachemiss":                        26,
		"total.num.dnscrypt.cert":                    0,
		"total.num.dnscrypt.cleartext":               0,
		"total.num.dnscrypt.crypted":                 0,
		"total.num.dnscrypt.malformed":               0,
		"total.num.prefetch":                         0,
		"total.num.expired":                          0,
		"total.num.queries":                          360,
		"total.num.queries_ip_ratelimited":           0,
		"total.num.recursivereplies":                 26,
		"total.num.zero_ttl":                         0,
		"total.recursion.time.avg":                   450,
		"total.recursion.time.median":                306,
		"total.requestlist.avg":                      192,
		"total.requestlist.current.all":              0,
		"total.requestlist.current.user":             0,
		"total.requestlist.exceeded":                 0,
		"total.requestlist.max":                      2,
		"total.requestlist.overwritten":              0,
		"total.tcpusage":                             0,
		"unwanted.queries":                           0,
		"unwanted.replies":                           0,
	}
)

var (
	expectedReset1 = map[string]int64{
		"dnscrypt_nonce.cache.count":                 0,
		"dnscrypt_shared_secret.cache.count":         0,
		"infra.cache.count":                          181,
		"key.cache.count":                            10,
		"mem.cache.dnscrypt_nonce":                   0,
		"mem.cache.dnscrypt_shared_secret":           0,
		"mem.cache.message":                          86064,
		"mem.cache.rrset":                            172757,
		"mem.mod.iterator":                           16588,
		"mem.mod.respip":                             0,
		"mem.mod.subnet":                             74504,
		"mem.mod.validator":                          79979,
		"mem.streamwait":                             0,
		"msg.cache.count":                            67,
		"num.answer.bogus":                           0,
		"num.answer.rcode.FORMERR":                   0,
		"num.answer.rcode.NOERROR":                   90,
		"num.answer.rcode.NOTIMPL":                   0,
		"num.answer.rcode.NXDOMAIN":                  10,
		"num.answer.rcode.REFUSED":                   0,
		"num.answer.rcode.SERVFAIL":                  0,
		"num.answer.rcode.nodata":                    10,
		"num.answer.secure":                          0,
		"num.query.aggressive.NOERROR":               2,
		"num.query.aggressive.NXDOMAIN":              0,
		"num.query.authzone.down":                    0,
		"num.query.authzone.up":                      0,
		"num.query.class.IN":                         100,
		"num.query.dnscrypt.replay":                  0,
		"num.query.dnscrypt.shared_secret.cachemiss": 0,
		"num.query.edns.DO":                          0,
		"num.query.edns.present":                     0,
		"num.query.flags.AA":                         0,
		"num.query.flags.AD":                         0,
		"num.query.flags.CD":                         0,
		"num.query.flags.QR":                         0,
		"num.query.flags.RA":                         0,
		"num.query.flags.RD":                         100,
		"num.query.flags.TC":                         0,
		"num.query.flags.Z":                          0,
		"num.query.ipv6":                             0,
		"num.query.opcode.QUERY":                     100,
		"num.query.ratelimited":                      0,
		"num.query.subnet":                           0,
		"num.query.subnet_cache":                     0,
		"num.query.tcp":                              0,
		"num.query.tcpout":                           1,
		"num.query.tls":                              0,
		"num.query.tls.resume":                       0,
		"num.query.type.A":                           30,
		"num.query.type.AAAA":                        30,
		"num.query.type.MX":                          30,
		"num.query.type.PTR":                         10,
		"num.rrset.bogus":                            0,
		"rrset.cache.count":                          303,
		"thread0.num.cachehits":                      44,
		"thread0.num.cachemiss":                      7,
		"thread0.num.dnscrypt.cert":                  0,
		"thread0.num.dnscrypt.cleartext":             0,
		"thread0.num.dnscrypt.crypted":               0,
		"thread0.num.dnscrypt.malformed":             0,
		"thread0.num.expired":                        0,
		"thread0.num.prefetch":                       0,
		"thread0.num.queries":                        51,
		"thread0.num.queries_ip_ratelimited":         0,
		"thread0.num.recursivereplies":               7,
		"thread0.num.zero_ttl":                       0,
		"thread0.recursion.time.avg":                 365,
		"thread0.recursion.time.median":              57,
		"thread0.requestlist.avg":                    0,
		"thread0.requestlist.current.all":            0,
		"thread0.requestlist.current.user":           0,
		"thread0.requestlist.exceeded":               0,
		"thread0.requestlist.max":                    0,
		"thread0.requestlist.overwritten":            0,
		"thread0.tcpusage":                           0,
		"thread1.num.cachehits":                      46,
		"thread1.num.cachemiss":                      3,
		"thread1.num.dnscrypt.cert":                  0,
		"thread1.num.dnscrypt.cleartext":             0,
		"thread1.num.dnscrypt.crypted":               0,
		"thread1.num.dnscrypt.malformed":             0,
		"thread1.num.prefetch":                       0,
		"thread1.num.expired":                        0,
		"thread1.num.queries":                        49,
		"thread1.num.queries_ip_ratelimited":         0,
		"thread1.num.recursivereplies":               3,
		"thread1.num.zero_ttl":                       0,
		"thread1.recursion.time.avg":                 1582,
		"thread1.recursion.time.median":              0,
		"thread1.requestlist.avg":                    0,
		"thread1.requestlist.current.all":            0,
		"thread1.requestlist.current.user":           0,
		"thread1.requestlist.exceeded":               0,
		"thread1.requestlist.max":                    0,
		"thread1.requestlist.overwritten":            0,
		"thread1.tcpusage":                           0,
		"time.elapsed":                               45,
		"time.now":                                   1574103644,
		"time.up":                                    45,
		"total.num.cachehits":                        90,
		"total.num.cachemiss":                        10,
		"total.num.dnscrypt.cert":                    0,
		"total.num.dnscrypt.cleartext":               0,
		"total.num.dnscrypt.crypted":                 0,
		"total.num.dnscrypt.malformed":               0,
		"total.num.prefetch":                         0,
		"total.num.expired":                          0,
		"total.num.queries":                          100,
		"total.num.queries_ip_ratelimited":           0,
		"total.num.recursivereplies":                 10,
		"total.num.zero_ttl":                         0,
		"total.recursion.time.avg":                   730,
		"total.recursion.time.median":                28,
		"total.requestlist.avg":                      0,
		"total.requestlist.current.all":              0,
		"total.requestlist.current.user":             0,
		"total.requestlist.exceeded":                 0,
		"total.requestlist.max":                      0,
		"total.requestlist.overwritten":              0,
		"total.tcpusage":                             0,
		"unwanted.queries":                           0,
		"unwanted.replies":                           0,
	}
	expectedReset2 = map[string]int64{
		"dnscrypt_nonce.cache.count":                 0,
		"dnscrypt_shared_secret.cache.count":         0,
		"infra.cache.count":                          181,
		"key.cache.count":                            10,
		"mem.cache.dnscrypt_nonce":                   0,
		"mem.cache.dnscrypt_shared_secret":           0,
		"mem.cache.message":                          86064,
		"mem.cache.rrset":                            172757,
		"mem.mod.iterator":                           16588,
		"mem.mod.respip":                             0,
		"mem.mod.subnet":                             74504,
		"mem.mod.validator":                          79979,
		"mem.streamwait":                             0,
		"msg.cache.count":                            67,
		"num.answer.bogus":                           0,
		"num.answer.rcode.FORMERR":                   0,
		"num.answer.rcode.NOERROR":                   0,
		"num.answer.rcode.NOTIMPL":                   0,
		"num.answer.rcode.NXDOMAIN":                  0,
		"num.answer.rcode.REFUSED":                   0,
		"num.answer.rcode.SERVFAIL":                  0,
		"num.answer.rcode.nodata":                    0,
		"num.answer.secure":                          0,
		"num.query.aggressive.NOERROR":               0,
		"num.query.aggressive.NXDOMAIN":              0,
		"num.query.authzone.down":                    0,
		"num.query.authzone.up":                      0,
		"num.query.class.IN":                         0,
		"num.query.dnscrypt.replay":                  0,
		"num.query.dnscrypt.shared_secret.cachemiss": 0,
		"num.query.edns.DO":                          0,
		"num.query.edns.present":                     0,
		"num.query.flags.AA":                         0,
		"num.query.flags.AD":                         0,
		"num.query.flags.CD":                         0,
		"num.query.flags.QR":                         0,
		"num.query.flags.RA":                         0,
		"num.query.flags.RD":                         0,
		"num.query.flags.TC":                         0,
		"num.query.flags.Z":                          0,
		"num.query.ipv6":                             0,
		"num.query.opcode.QUERY":                     0,
		"num.query.ratelimited":                      0,
		"num.query.subnet":                           0,
		"num.query.subnet_cache":                     0,
		"num.query.tcp":                              0,
		"num.query.tcpout":                           0,
		"num.query.tls":                              0,
		"num.query.tls.resume":                       0,
		"num.query.type.A":                           0,
		"num.query.type.AAAA":                        0,
		"num.query.type.MX":                          0,
		"num.query.type.PTR":                         0,
		"num.rrset.bogus":                            0,
		"rrset.cache.count":                          303,
		"thread0.num.cachehits":                      0,
		"thread0.num.cachemiss":                      0,
		"thread0.num.dnscrypt.cert":                  0,
		"thread0.num.dnscrypt.cleartext":             0,
		"thread0.num.dnscrypt.crypted":               0,
		"thread0.num.dnscrypt.malformed":             0,
		"thread0.num.expired":                        0,
		"thread0.num.prefetch":                       0,
		"thread0.num.queries":                        0,
		"thread0.num.queries_ip_ratelimited":         0,
		"thread0.num.recursivereplies":               0,
		"thread0.num.zero_ttl":                       0,
		"thread0.recursion.time.avg":                 0,
		"thread0.recursion.time.median":              0,
		"thread0.requestlist.avg":                    0,
		"thread0.requestlist.current.all":            0,
		"thread0.requestlist.current.user":           0,
		"thread0.requestlist.exceeded":               0,
		"thread0.requestlist.max":                    0,
		"thread0.requestlist.overwritten":            0,
		"thread0.tcpusage":                           0,
		"thread1.num.cachehits":                      0,
		"thread1.num.cachemiss":                      0,
		"thread1.num.dnscrypt.cert":                  0,
		"thread1.num.dnscrypt.cleartext":             0,
		"thread1.num.dnscrypt.crypted":               0,
		"thread1.num.dnscrypt.malformed":             0,
		"thread1.num.prefetch":                       0,
		"thread1.num.expired":                        0,
		"thread1.num.queries":                        0,
		"thread1.num.queries_ip_ratelimited":         0,
		"thread1.num.recursivereplies":               0,
		"thread1.num.zero_ttl":                       0,
		"thread1.recursion.time.avg":                 0,
		"thread1.recursion.time.median":              0,
		"thread1.requestlist.avg":                    0,
		"thread1.requestlist.current.all":            0,
		"thread1.requestlist.current.user":           0,
		"thread1.requestlist.exceeded":               0,
		"thread1.requestlist.max":                    0,
		"thread1.requestlist.overwritten":            0,
		"thread1.tcpusage":                           0,
		"time.elapsed":                               26,
		"time.now":                                   1574103671,
		"time.up":                                    71,
		"total.num.cachehits":                        0,
		"total.num.cachemiss":                        0,
		"total.num.dnscrypt.cert":                    0,
		"total.num.dnscrypt.cleartext":               0,
		"total.num.dnscrypt.crypted":                 0,
		"total.num.dnscrypt.malformed":               0,
		"total.num.prefetch":                         0,
		"total.num.expired":                          0,
		"total.num.queries":                          0,
		"total.num.queries_ip_ratelimited":           0,
		"total.num.recursivereplies":                 0,
		"total.num.zero_ttl":                         0,
		"total.recursion.time.avg":                   0,
		"total.recursion.time.median":                0,
		"total.requestlist.avg":                      0,
		"total.requestlist.current.all":              0,
		"total.requestlist.current.user":             0,
		"total.requestlist.exceeded":                 0,
		"total.requestlist.max":                      0,
		"total.requestlist.overwritten":              0,
		"total.tcpusage":                             0,
		"unwanted.queries":                           0,
		"unwanted.replies":                           0,
	}

	expectedReset3 = map[string]int64{
		"dnscrypt_nonce.cache.count":                 0,
		"dnscrypt_shared_secret.cache.count":         0,
		"infra.cache.count":                          303,
		"key.cache.count":                            15,
		"mem.cache.dnscrypt_nonce":                   0,
		"mem.cache.dnscrypt_shared_secret":           0,
		"mem.cache.message":                          105471,
		"mem.cache.rrset":                            235917,
		"mem.mod.iterator":                           16588,
		"mem.mod.respip":                             0,
		"mem.mod.subnet":                             74504,
		"mem.mod.validator":                          87270,
		"mem.streamwait":                             0,
		"msg.cache.count":                            127,
		"num.answer.bogus":                           0,
		"num.answer.rcode.FORMERR":                   0,
		"num.answer.rcode.NOERROR":                   60,
		"num.answer.rcode.NOTIMPL":                   0,
		"num.answer.rcode.NXDOMAIN":                  10,
		"num.answer.rcode.REFUSED":                   0,
		"num.answer.rcode.SERVFAIL":                  0,
		"num.answer.rcode.nodata":                    10,
		"num.answer.secure":                          0,
		"num.query.aggressive.NOERROR":               2,
		"num.query.aggressive.NXDOMAIN":              0,
		"num.query.authzone.down":                    0,
		"num.query.authzone.up":                      0,
		"num.query.class.IN":                         70,
		"num.query.dnscrypt.replay":                  0,
		"num.query.dnscrypt.shared_secret.cachemiss": 0,
		"num.query.edns.DO":                          0,
		"num.query.edns.present":                     0,
		"num.query.flags.AA":                         0,
		"num.query.flags.AD":                         0,
		"num.query.flags.CD":                         0,
		"num.query.flags.QR":                         0,
		"num.query.flags.RA":                         0,
		"num.query.flags.RD":                         70,
		"num.query.flags.TC":                         0,
		"num.query.flags.Z":                          0,
		"num.query.ipv6":                             0,
		"num.query.opcode.QUERY":                     70,
		"num.query.ratelimited":                      0,
		"num.query.subnet":                           0,
		"num.query.subnet_cache":                     0,
		"num.query.tcp":                              0,
		"num.query.tcpout":                           0,
		"num.query.tls":                              0,
		"num.query.tls.resume":                       0,
		"num.query.type.A":                           20,
		"num.query.type.AAAA":                        20,
		"num.query.type.MX":                          20,
		"num.query.type.PTR":                         10,
		"num.rrset.bogus":                            0,
		"rrset.cache.count":                          501,
		"thread0.num.cachehits":                      30,
		"thread0.num.cachemiss":                      4,
		"thread0.num.dnscrypt.cert":                  0,
		"thread0.num.dnscrypt.cleartext":             0,
		"thread0.num.dnscrypt.crypted":               0,
		"thread0.num.dnscrypt.malformed":             0,
		"thread0.num.expired":                        0,
		"thread0.num.prefetch":                       0,
		"thread0.num.queries":                        34,
		"thread0.num.queries_ip_ratelimited":         0,
		"thread0.num.recursivereplies":               4,
		"thread0.num.zero_ttl":                       0,
		"thread0.recursion.time.avg":                 541,
		"thread0.recursion.time.median":              98,
		"thread0.requestlist.avg":                    0,
		"thread0.requestlist.current.all":            0,
		"thread0.requestlist.current.user":           0,
		"thread0.requestlist.exceeded":               0,
		"thread0.requestlist.max":                    0,
		"thread0.requestlist.overwritten":            0,
		"thread0.tcpusage":                           0,
		"thread1.num.cachehits":                      33,
		"thread1.num.cachemiss":                      3,
		"thread1.num.dnscrypt.cert":                  0,
		"thread1.num.dnscrypt.cleartext":             0,
		"thread1.num.dnscrypt.crypted":               0,
		"thread1.num.dnscrypt.malformed":             0,
		"thread1.num.prefetch":                       0,
		"thread1.num.expired":                        0,
		"thread1.num.queries":                        36,
		"thread1.num.queries_ip_ratelimited":         0,
		"thread1.num.recursivereplies":               3,
		"thread1.num.zero_ttl":                       0,
		"thread1.recursion.time.avg":                 62,
		"thread1.recursion.time.median":              0,
		"thread1.requestlist.avg":                    1666,
		"thread1.requestlist.current.all":            0,
		"thread1.requestlist.current.user":           0,
		"thread1.requestlist.exceeded":               0,
		"thread1.requestlist.max":                    5,
		"thread1.requestlist.overwritten":            0,
		"thread1.tcpusage":                           0,
		"time.elapsed":                               59,
		"time.now":                                   1574103731,
		"time.up":                                    131,
		"total.num.cachehits":                        63,
		"total.num.cachemiss":                        7,
		"total.num.dnscrypt.cert":                    0,
		"total.num.dnscrypt.cleartext":               0,
		"total.num.dnscrypt.crypted":                 0,
		"total.num.dnscrypt.malformed":               0,
		"total.num.prefetch":                         0,
		"total.num.expired":                          0,
		"total.num.queries":                          70,
		"total.num.queries_ip_ratelimited":           0,
		"total.num.recursivereplies":                 7,
		"total.num.zero_ttl":                         0,
		"total.recursion.time.avg":                   336,
		"total.recursion.time.median":                49,
		"total.requestlist.avg":                      714,
		"total.requestlist.current.all":              0,
		"total.requestlist.current.user":             0,
		"total.requestlist.exceeded":                 0,
		"total.requestlist.max":                      5,
		"total.requestlist.overwritten":              0,
		"total.tcpusage":                             0,
		"unwanted.queries":                           0,
		"unwanted.replies":                           0,
	}
)
