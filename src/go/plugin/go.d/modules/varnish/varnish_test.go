// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
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

	dataVarnishStat, _ = os.ReadFile("testdata/varnishstat.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":  dataConfigJSON,
		"dataConfigYAML":  dataConfigYAML,
		"dataVarnishStat": dataVarnishStat,
	} {
		require.NotNil(t, data, name)
	}
}

func TestVarnish_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Varnish{}, dataConfigJSON, dataConfigYAML)
}

func TestVarnish_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if 'binary_path' is not set": {
			wantFail: true,
			config: Config{
				BinaryPath: "",
			},
		},
		"fails if failed to find binary": {
			wantFail: true,
			config: Config{
				BinaryPath: "varnishstat!!!",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := New()
			pf.Config = test.config

			if test.wantFail {
				assert.Error(t, pf.Init())
			} else {
				assert.NoError(t, pf.Init())
			}
		})
	}
}

func TestVarnish_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Varnish
	}{
		"not initialized exec": {
			prepare: func() *Varnish {
				return New()
			},
		},
		"after check": {
			prepare: func() *Varnish {
				v := New()
				v.exec = prepareMockOk()
				_ = v.Check()
				return v
			},
		},
		"after collect": {
			prepare: func() *Varnish {
				v := New()
				v.exec = prepareMockOk()
				_ = v.Collect()
				return v
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := test.prepare()

			assert.NotPanics(t, pf.Cleanup)
		})
	}
}

func TestVarnish_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestVarnish_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockVarnishStatExec
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"error on varnishstat call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnVarnishStatCall,
		},
		"unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			v := New()
			v.exec = test.prepareMock()

			if test.wantFail {
				assert.Error(t, v.Check())
			} else {
				assert.NoError(t, v.Check())
			}
		})
	}
}

func TestVarnish_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockVarnishStatExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case": {
			prepareMock: prepareMockOk,
			wantCharts:  18,
			wantMetrics: map[string]int64{
				"Transient_c_bytes":             0,
				"Transient_c_fail":              0,
				"Transient_c_freed":             0,
				"Transient_c_req":               0,
				"Transient_g_alloc":             0,
				"Transient_g_bytes":             0,
				"Transient_g_space":             0,
				"backend_busy":                  0,
				"backend_conn":                  0,
				"backend_fail":                  0,
				"backend_recycle":               0,
				"backend_req":                   0,
				"backend_retry":                 0,
				"backend_reuse":                 0,
				"backend_unhealthy":             0,
				"ban.creat":                     1,
				"ban.dbg_busy":                  0,
				"ban.dbg_try_fail":              0,
				"ban.destroy":                   0,
				"ban.locks":                     70,
				"bans":                          1,
				"bans_added":                    1,
				"bans_completed":                1,
				"bans_deleted":                  0,
				"bans_dups":                     0,
				"bans_lurker_contention":        0,
				"bans_lurker_obj_killed":        0,
				"bans_lurker_obj_killed_cutoff": 0,
				"bans_lurker_tested":            0,
				"bans_lurker_tests_tested":      0,
				"bans_obj":                      0,
				"bans_obj_killed":               0,
				"bans_persisted_bytes":          16,
				"bans_persisted_fragmentation":  0,
				"bans_req":                      0,
				"bans_tested":                   0,
				"bans_tests_tested":             0,
				"beresp_shortlived":             0,
				"beresp_uncacheable":            0,
				"busy_killed":                   0,
				"busy_sleep":                    0,
				"busy_wakeup":                   0,
				"busyobj.allocs":                0,
				"busyobj.creat":                 0,
				"busyobj.dbg_busy":              0,
				"busyobj.dbg_try_fail":          0,
				"busyobj.destroy":               0,
				"busyobj.frees":                 0,
				"busyobj.live":                  0,
				"busyobj.locks":                 0,
				"busyobj.pool":                  10,
				"busyobj.randry":                0,
				"busyobj.recycle":               0,
				"busyobj.surplus":               0,
				"busyobj.sz_actual":             98272,
				"busyobj.sz_wanted":             98304,
				"busyobj.timeout":               0,
				"busyobj.toosmall":              0,
				"cache_hit":                     0,
				"cache_hit_grace":               0,
				"cache_hitmiss":                 0,
				"cache_hitpass":                 0,
				"cache_miss":                    0,
				"child_died":                    0,
				"child_dump":                    0,
				"child_exit":                    0,
				"child_panic":                   0,
				"child_start":                   1,
				"child_stop":                    0,
				"cli.creat":                     1,
				"cli.dbg_busy":                  0,
				"cli.dbg_try_fail":              0,
				"cli.destroy":                   0,
				"cli.locks":                     535,
				"client_req":                    0,
				"client_req_400":                0,
				"client_req_417":                0,
				"client_resp_500":               0,
				"conn_pool.creat":               2,
				"conn_pool.dbg_busy":            0,
				"conn_pool.dbg_try_fail":        0,
				"conn_pool.destroy":             0,
				"conn_pool.locks":               1,
				"default_bereq_bodybytes":       0,
				"default_bereq_hdrbytes":        0,
				"default_beresp_bodybytes":      0,
				"default_beresp_hdrbytes":       0,
				"default_busy":                  0,
				"default_conn":                  0,
				"default_fail":                  0,
				"default_fail_eacces":           0,
				"default_fail_eaddrnotavail":    0,
				"default_fail_econnrefused":     0,
				"default_fail_enetunreach":      0,
				"default_fail_etimedout":        0,
				"default_fail_other":            0,
				"default_happy":                 0,
				"default_helddown":              0,
				"default_pipe_hdrbytes":         0,
				"default_pipe_in":               0,
				"default_pipe_out":              0,
				"default_req":                   0,
				"default_unhealthy":             0,
				"director.creat":                1,
				"director.dbg_busy":             0,
				"director.dbg_try_fail":         0,
				"director.destroy":              0,
				"director.locks":                0,
				"esi_errors":                    0,
				"esi_req":                       0,
				"esi_warnings":                  0,
				"exp.creat":                     1,
				"exp.dbg_busy":                  0,
				"exp.dbg_try_fail":              0,
				"exp.destroy":                   0,
				"exp.locks":                     501,
				"exp_mailed":                    0,
				"exp_received":                  0,
				"fetch_1xx":                     0,
				"fetch_204":                     0,
				"fetch_304":                     0,
				"fetch_bad":                     0,
				"fetch_chunked":                 0,
				"fetch_eof":                     0,
				"fetch_failed":                  0,
				"fetch_head":                    0,
				"fetch_length":                  0,
				"fetch_no_thread":               0,
				"fetch_none":                    0,
				"hcb.creat":                     1,
				"hcb.dbg_busy":                  0,
				"hcb.dbg_try_fail":              0,
				"hcb.destroy":                   0,
				"hcb.locks":                     9,
				"hcb_insert":                    0,
				"hcb_lock":                      0,
				"hcb_nolock":                    0,
				"losthdr":                       0,
				"lru.creat":                     2,
				"lru.dbg_busy":                  0,
				"lru.dbg_try_fail":              0,
				"lru.destroy":                   0,
				"lru.locks":                     0,
				"mempool.creat":                 5,
				"mempool.dbg_busy":              0,
				"mempool.dbg_try_fail":          0,
				"mempool.destroy":               0,
				"mempool.locks":                 6989,
				"n_backend":                     1,
				"n_expired":                     0,
				"n_gunzip":                      0,
				"n_gzip":                        0,
				"n_lru_limited":                 0,
				"n_lru_moved":                   0,
				"n_lru_nuked":                   0,
				"n_obj_purged":                  0,
				"n_object":                      0,
				"n_objectcore":                  0,
				"n_objecthead":                  0,
				"n_pipe":                        0,
				"n_purges":                      0,
				"n_test_gunzip":                 0,
				"n_vampireobject":               0,
				"n_vcl":                         1,
				"n_vcl_avail":                   1,
				"n_vcl_discard":                 0,
				"objhdr.creat":                  1,
				"objhdr.dbg_busy":               0,
				"objhdr.dbg_try_fail":           0,
				"objhdr.destroy":                0,
				"objhdr.locks":                  0,
				"perpool.creat":                 2,
				"perpool.dbg_busy":              0,
				"perpool.dbg_try_fail":          0,
				"perpool.destroy":               0,
				"perpool.locks":                 216,
				"pipe_limited":                  0,
				"pipestat.creat":                1,
				"pipestat.dbg_busy":             0,
				"pipestat.dbg_try_fail":         0,
				"pipestat.destroy":              0,
				"pipestat.locks":                0,
				"pools":                         2,
				"probe.creat":                   1,
				"probe.dbg_busy":                0,
				"probe.dbg_try_fail":            0,
				"probe.destroy":                 0,
				"probe.locks":                   1,
				"req0.allocs":                   0,
				"req0.frees":                    0,
				"req0.live":                     0,
				"req0.pool":                     10,
				"req0.randry":                   0,
				"req0.recycle":                  0,
				"req0.surplus":                  0,
				"req0.sz_actual":                98272,
				"req0.sz_wanted":                98304,
				"req0.timeout":                  0,
				"req0.toosmall":                 0,
				"req1.allocs":                   0,
				"req1.frees":                    0,
				"req1.live":                     0,
				"req1.pool":                     10,
				"req1.randry":                   0,
				"req1.recycle":                  0,
				"req1.surplus":                  0,
				"req1.sz_actual":                98272,
				"req1.sz_wanted":                98304,
				"req1.timeout":                  0,
				"req1.toosmall":                 0,
				"req_dropped":                   0,
				"s0_c_bytes":                    0,
				"s0_c_fail":                     0,
				"s0_c_freed":                    0,
				"s0_c_req":                      0,
				"s0_g_alloc":                    0,
				"s0_g_bytes":                    0,
				"s0_g_space":                    268435456,
				"s_bgfetch":                     0,
				"s_fetch":                       0,
				"s_pass":                        0,
				"s_pipe":                        0,
				"s_pipe_hdrbytes":               0,
				"s_pipe_in":                     0,
				"s_pipe_out":                    0,
				"s_req_bodybytes":               0,
				"s_req_hdrbytes":                0,
				"s_resp_bodybytes":              0,
				"s_resp_hdrbytes":               0,
				"s_sess":                        0,
				"s_synth":                       0,
				"sc_overload":                   0,
				"sc_pipe_overflow":              0,
				"sc_range_short":                0,
				"sc_rem_close":                  0,
				"sc_req_close":                  0,
				"sc_req_http10":                 0,
				"sc_req_http20":                 0,
				"sc_resp_close":                 0,
				"sc_rx_bad":                     0,
				"sc_rx_body":                    0,
				"sc_rx_close_idle":              0,
				"sc_rx_junk":                    0,
				"sc_rx_overflow":                0,
				"sc_rx_timeout":                 0,
				"sc_tx_eof":                     0,
				"sc_tx_error":                   0,
				"sc_tx_pipe":                    0,
				"sc_vcl_failure":                0,
				"sess.creat":                    0,
				"sess.dbg_busy":                 0,
				"sess.dbg_try_fail":             0,
				"sess.destroy":                  0,
				"sess.locks":                    0,
				"sess0.allocs":                  0,
				"sess0.frees":                   0,
				"sess0.live":                    0,
				"sess0.pool":                    10,
				"sess0.randry":                  0,
				"sess0.recycle":                 0,
				"sess0.surplus":                 0,
				"sess0.sz_actual":               736,
				"sess0.sz_wanted":               768,
				"sess0.timeout":                 0,
				"sess0.toosmall":                0,
				"sess1.allocs":                  0,
				"sess1.frees":                   0,
				"sess1.live":                    0,
				"sess1.pool":                    10,
				"sess1.randry":                  0,
				"sess1.recycle":                 0,
				"sess1.surplus":                 0,
				"sess1.sz_actual":               736,
				"sess1.sz_wanted":               768,
				"sess1.timeout":                 0,
				"sess1.toosmall":                0,
				"sess_closed":                   0,
				"sess_closed_err":               0,
				"sess_conn":                     0,
				"sess_dropped":                  0,
				"sess_fail":                     0,
				"sess_fail_ebadf":               0,
				"sess_fail_econnaborted":        0,
				"sess_fail_eintr":               0,
				"sess_fail_emfile":              0,
				"sess_fail_enomem":              0,
				"sess_fail_other":               0,
				"sess_herd":                     0,
				"sess_queued":                   0,
				"sess_readahead":                0,
				"shm_cont":                      0,
				"shm_cycles":                    0,
				"shm_flushes":                   0,
				"shm_records":                   1050,
				"shm_writes":                    1050,
				"sma.creat":                     2,
				"sma.dbg_busy":                  0,
				"sma.dbg_try_fail":              0,
				"sma.destroy":                   0,
				"sma.locks":                     0,
				"summs":                         0,
				"thread_queue_len":              0,
				"threads":                       200,
				"threads_created":               200,
				"threads_destroyed":             0,
				"threads_failed":                0,
				"threads_limited":               0,
				"uptime":                        1569,
				"vbe.creat":                     1,
				"vbe.dbg_busy":                  0,
				"vbe.dbg_try_fail":              0,
				"vbe.destroy":                   0,
				"vbe.locks":                     1,
				"vcapace.creat":                 1,
				"vcapace.dbg_busy":              0,
				"vcapace.dbg_try_fail":          0,
				"vcapace.destroy":               0,
				"vcapace.locks":                 0,
				"vcl.creat":                     1,
				"vcl.dbg_busy":                  0,
				"vcl.dbg_try_fail":              0,
				"vcl.destroy":                   0,
				"vcl.locks":                     3,
				"vcl_fail":                      0,
				"vmods":                         0,
				"vxid.creat":                    1,
				"vxid.dbg_busy":                 0,
				"vxid.dbg_try_fail":             0,
				"vxid.destroy":                  0,
				"vxid.locks":                    0,
				"waiter.creat":                  2,
				"waiter.dbg_busy":               0,
				"waiter.dbg_try_fail":           0,
				"waiter.destroy":                0,
				"waiter.locks":                  32,
				"wq.creat":                      1,
				"wq.dbg_busy":                   0,
				"wq.dbg_try_fail":               0,
				"wq.destroy":                    0,
				"wq.locks":                      1771,
				"ws_backend_overflow":           0,
				"ws_client_overflow":            0,
				"ws_session_overflow":           0,
				"ws_thread_overflow":            0,
				"wstat.creat":                   1,
				"wstat.dbg_busy":                0,
				"wstat.dbg_try_fail":            0,
				"wstat.destroy":                 0,
				"wstat.locks":                   541,
			},
		},
		"error on varnishstat call": {
			prepareMock: prepareMockErrOnVarnishStatCall,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			v := New()
			v.exec = test.prepareMock()

			mx := v.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantCharts, len(*v.Charts()))
				module.TestMetricsHasAllChartsDims(t, v.Charts(), mx)
			}
		})
	}
}

func prepareMockOk() *mockVarnishStatExec {
	return &mockVarnishStatExec{
		varnishStatisticsData: dataVarnishStat,
	}
}

func prepareMockErrOnVarnishStatCall() *mockVarnishStatExec {
	return &mockVarnishStatExec{
		varnishStatisticsData: nil,
		errOnVarnishStatCall:  true,
	}
}

func prepareMockUnexpectedResponse() *mockVarnishStatExec {
	return &mockVarnishStatExec{
		varnishStatisticsData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockVarnishStatExec struct {
	errOnVarnishStatCall  bool
	varnishStatisticsData []byte
}

func (m *mockVarnishStatExec) varnishStatistics() ([]byte, error) {

	if m.errOnVarnishStatCall {
		return nil, errors.New("mock varnishStatistics() error")
	}

	return m.varnishStatisticsData, nil
}
