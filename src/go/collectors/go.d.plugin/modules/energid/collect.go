// SPDX-License-Identifier: GPL-3.0-or-later

package energid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/go.d.plugin/pkg/stm"
	"github.com/netdata/go.d.plugin/pkg/web"
)

const (
	jsonRPCVersion = "1.1"

	methodGetBlockchainInfo = "getblockchaininfo"
	methodGetMemPoolInfo    = "getmempoolinfo"
	methodGetNetworkInfo    = "getnetworkinfo"
	methodGetTXOutSetInfo   = "gettxoutsetinfo"
	methodGetMemoryInfo     = "getmemoryinfo"
)

var infoRequests = rpcRequests{
	{JSONRPC: jsonRPCVersion, ID: 1, Method: methodGetBlockchainInfo},
	{JSONRPC: jsonRPCVersion, ID: 2, Method: methodGetMemPoolInfo},
	{JSONRPC: jsonRPCVersion, ID: 3, Method: methodGetNetworkInfo},
	{JSONRPC: jsonRPCVersion, ID: 4, Method: methodGetTXOutSetInfo},
	{JSONRPC: jsonRPCVersion, ID: 5, Method: methodGetMemoryInfo},
}

func (e *Energid) collect() (map[string]int64, error) {
	responses, err := e.scrapeEnergid(infoRequests)
	if err != nil {
		return nil, err
	}

	info, err := e.collectInfoResponse(infoRequests, responses)
	if err != nil {
		return nil, err
	}

	return stm.ToMap(info), nil
}

func (e *Energid) collectInfoResponse(requests rpcRequests, responses rpcResponses) (*energidInfo, error) {
	var info energidInfo
	for _, req := range requests {
		resp := responses.getByID(req.ID)
		if resp == nil {
			e.Warningf("method '%s' (id %d) not in responses", req.Method, req.ID)
			continue
		}

		if resp.Error != nil {
			e.Warningf("server returned an error on method '%s': %v", req.Method, resp.Error)
			continue
		}

		var err error
		switch req.Method {
		case methodGetBlockchainInfo:
			info.Blockchain, err = parseBlockchainInfo(resp.Result)
		case methodGetMemPoolInfo:
			info.MemPool, err = parseMemPoolInfo(resp.Result)
		case methodGetNetworkInfo:
			info.Network, err = parseNetworkInfo(resp.Result)
		case methodGetTXOutSetInfo:
			info.TxOutSet, err = parseTXOutSetInfo(resp.Result)
		case methodGetMemoryInfo:
			info.Memory, err = parseMemoryInfo(resp.Result)
		}
		if err != nil {
			return nil, fmt.Errorf("parse '%s' method result: %v", req.Method, err)
		}
	}

	return &info, nil
}

func parseBlockchainInfo(result []byte) (*blockchainInfo, error) {
	var m blockchainInfo
	if err := json.Unmarshal(result, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func parseMemPoolInfo(result []byte) (*memPoolInfo, error) {
	var m memPoolInfo
	if err := json.Unmarshal(result, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func parseNetworkInfo(result []byte) (*networkInfo, error) {
	var m networkInfo
	if err := json.Unmarshal(result, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func parseTXOutSetInfo(result []byte) (*txOutSetInfo, error) {
	var m txOutSetInfo
	if err := json.Unmarshal(result, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func parseMemoryInfo(result []byte) (*memoryInfo, error) {
	var m memoryInfo
	if err := json.Unmarshal(result, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (e *Energid) scrapeEnergid(requests rpcRequests) (rpcResponses, error) {
	req, _ := web.NewHTTPRequest(e.Request)
	req.Method = http.MethodPost
	req.Header.Set("Content-Type", "application/json")
	body, _ := json.Marshal(requests)
	req.Body = io.NopCloser(bytes.NewReader(body))

	var resp rpcResponses
	if err := e.doOKDecode(req, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func (e *Energid) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}

	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
