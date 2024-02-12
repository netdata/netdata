// SPDX-License-Identifier: GPL-3.0-or-later

package energid

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	v241GetBlockchainInfo, _ = os.ReadFile("testdata/v2.4.1/getblockchaininfo.json")
	v241GetMemPoolInfo, _    = os.ReadFile("testdata/v2.4.1/getmempoolinfo.json")
	v241GetNetworkInfo, _    = os.ReadFile("testdata/v2.4.1/getnetworkinfo.json")
	v241GetTXOutSetInfo, _   = os.ReadFile("testdata/v2.4.1/gettxoutsetinfo.json")
	v241GetMemoryInfo, _     = os.ReadFile("testdata/v2.4.1/getmemoryinfo.json")
)

func Test_Testdata(t *testing.T) {
	for name, data := range map[string][]byte{
		"v241GetBlockchainInfo": v241GetBlockchainInfo,
		"v241GetMemPoolInfo":    v241GetMemPoolInfo,
		"v241GetNetworkInfo":    v241GetNetworkInfo,
		"v241GetTXOutSetInfo":   v241GetTXOutSetInfo,
		"v241GetMemoryInfo":     v241GetMemoryInfo,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*Energid)(nil), New())
}

func Test_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset URL": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Request: web.Request{URL: ""},
				},
			},
		},
		"fails on invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Request: web.Request{
						URL: "http://127.0.0.1:38001",
					},
					Client: web.Client{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			energid := New()
			energid.Config = test.config

			if test.wantFail {
				assert.False(t, energid.Init())
			} else {
				assert.True(t, energid.Init())
			}
		})
	}
}

func Test_Charts(t *testing.T) {
	energid := New()
	require.True(t, energid.Init())
	assert.NotNil(t, energid.Charts())
}

func Test_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func Test_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (energid *Energid, cleanup func())
		wantFail bool
	}{
		"success on valid v2.4.1 response": {
			prepare: prepareEnergidV241,
		},
		"fails on 404 response": {
			wantFail: true,
			prepare:  prepareEnergid404,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  prepareEnergidConnectionRefused,
		},
		"fails on response with invalid data": {
			wantFail: true,
			prepare:  prepareEnergidInvalidData,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			energid, cleanup := test.prepare()
			defer cleanup()

			require.True(t, energid.Init())

			if test.wantFail {
				assert.False(t, energid.Check())
			} else {
				assert.True(t, energid.Check())
			}
		})
	}
}

func Test_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() (energid *Energid, cleanup func())
		wantCollected map[string]int64
	}{
		"success on valid v2.4.1 response": {
			prepare: prepareEnergidV241,
			wantCollected: map[string]int64{
				"blockchain_blocks":        1,
				"blockchain_difficulty":    0,
				"blockchain_headers":       1,
				"mempool_current":          1,
				"mempool_max":              300000000,
				"mempool_txsize":           1,
				"network_connections":      1,
				"network_timeoffset":       1,
				"secmem_free":              65248,
				"secmem_locked":            65536,
				"secmem_total":             65536,
				"secmem_used":              288,
				"utxo_output_transactions": 1,
				"utxo_transactions":        1,
			},
		},
		"fails on 404 response": {
			prepare: prepareEnergid404,
		},
		"fails on connection refused": {
			prepare: prepareEnergidConnectionRefused,
		},
		"fails on response with invalid data": {
			prepare: prepareEnergidInvalidData,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			energid, cleanup := test.prepare()
			defer cleanup()
			require.True(t, energid.Init())

			collected := energid.Collect()

			assert.Equal(t, test.wantCollected, collected)
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, energid, collected)
			}
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, energid *Energid, ms map[string]int64) {
	for _, chart := range *energid.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := ms[dim.ID]
			assert.Truef(t, ok, "chart '%s' dim '%s': no dim in collected", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := ms[v.ID]
			assert.Truef(t, ok, "chart '%s' dim '%s': no dim in collected", v.ID, chart.ID)
		}
	}
}

func prepareEnergidV241() (*Energid, func()) {
	srv := prepareEnergidEndPoint()
	energid := New()
	energid.URL = srv.URL

	return energid, srv.Close
}

func prepareEnergidInvalidData() (*Energid, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Hello world!"))
		}))
	energid := New()
	energid.URL = srv.URL

	return energid, srv.Close
}

func prepareEnergid404() (*Energid, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	energid := New()
	energid.URL = srv.URL

	return energid, srv.Close
}

func prepareEnergidConnectionRefused() (*Energid, func()) {
	energid := New()
	energid.URL = "http://127.0.0.1:38001"

	return energid, func() {}
}

func prepareEnergidEndPoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			body, _ := io.ReadAll(r.Body)
			var requests rpcRequests
			if err := json.Unmarshal(body, &requests); err != nil || len(requests) == 0 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			var responses rpcResponses
			for _, req := range requests {
				resp := rpcResponse{JSONRPC: jsonRPCVersion, ID: req.ID}
				switch req.Method {
				case methodGetBlockchainInfo:
					resp.Result = prepareResult(v241GetBlockchainInfo)
				case methodGetMemPoolInfo:
					resp.Result = prepareResult(v241GetMemPoolInfo)
				case methodGetNetworkInfo:
					resp.Result = prepareResult(v241GetNetworkInfo)
				case methodGetTXOutSetInfo:
					resp.Result = prepareResult(v241GetTXOutSetInfo)
				case methodGetMemoryInfo:
					resp.Result = prepareResult(v241GetMemoryInfo)
				default:
					resp.Error = &rpcError{Code: -32601, Message: "Method not found"}
				}
				responses = append(responses, resp)
			}

			bs, _ := json.Marshal(responses)
			_, _ = w.Write(bs)
		}))
}

func prepareResult(resp []byte) json.RawMessage {
	var r rpcResponse
	_ = json.Unmarshal(resp, &r)
	return r.Result
}
