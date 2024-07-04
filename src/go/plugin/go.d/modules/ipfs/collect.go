// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type ipfsBW struct {
	TotalIn  int64 `json:"TotalIn" stm:"TotalIn,8,1000"`
	TotalOut int64 `json:"TotalOut" stm:"TotalOut,8,1000"`
}

type ipfsPeers struct {
	Peers []interface{} `json:"Peers"`
}

type ipfsPins struct {
	Keys map[string]interface{} `json:"Keys"`
}

type ipfsRepo struct {
	RepoSize   int64 `json:"RepoSize" stm:"RepoSize"`
	StorageMax int64 `json:"StorageMax" stm:"StorageMax"`
	NumObjects int64 `json:"NumObjects" stm:"NumObjects"`
	// RepoPath   string `json:"RepoPath" stm:"RepoPath"`
	// Version    string `json:"Version" stm:"Version"`
}

func (ipfs *IPFS) collect() (map[string]int64, error) {
	statsBw, err := ipfs.requestIPFSBW()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(statsBw)

	statsPeers, err := ipfs.requestIPFSPeers()
	if err != nil {
		return nil, err
	}
	mx["peers"] = int64(len(statsPeers.Peers))

	if !ipfs.Pinapi && !ipfs.Repoapi {
		ipfs.charts.Remove("repo_objects")
	} else {
		statsPins, err := ipfs.requestIPFSPinapi()
		if err != nil {
			return nil, err
		}
		mx["pinned"] = int64(len(statsPins.Keys))

		count := 0
		for _, v := range statsPins.Keys {
			if keyData, ok := v.(map[string]interface{}); ok {
				if keyType, ok := keyData["Type"].(string); ok && keyType == "recursive" {
					count++
				}
			}
		}
		mx["recursive_pins"] = int64(count)

	}

	if !ipfs.Repoapi {
		ipfs.charts.Remove("repo_size")

	} else {
		statsRepo, err := ipfs.requestIPFSRepoapi()
		if err != nil {
			return nil, err
		}

		mx["avail"] = int64(statsRepo.StorageMax)
		mx["size"] = int64(statsRepo.RepoSize)
		mx["objects"] = int64(statsRepo.NumObjects)
	}

	return mx, nil
}

func (ipfs *IPFS) requestIPFSBW() (*ipfsBW, error) {
	req, err := web.NewHTTPRequest(ipfs.Request)
	if err != nil {
		return nil, err
	}

	req.URL.Path = "/api/v0/stats/bw"
	req.Method = http.MethodPost

	var stats ipfsBW
	if err := ipfs.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	if &(stats.TotalIn) == nil || &(stats.TotalOut) == nil {
		return nil, fmt.Errorf("unexpected response: not ipfs data")
	}

	return &stats, nil
}

func (ipfs *IPFS) requestIPFSPeers() (*ipfsPeers, error) {
	req, err := web.NewHTTPRequest(ipfs.Request)
	if err != nil {
		return nil, err
	}

	req.URL.Path = "/api/v0/swarm/peers"
	req.Method = http.MethodPost

	var stats ipfsPeers
	if err := ipfs.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (ipfs *IPFS) requestIPFSPinapi() (*ipfsPins, error) {
	req, err := web.NewHTTPRequest(ipfs.Request)
	if err != nil {
		return nil, err
	}

	req.URL.Path = "/api/v0/pin/ls"
	req.Method = http.MethodPost

	var stats ipfsPins
	if err := ipfs.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (ipfs *IPFS) requestIPFSRepoapi() (*ipfsRepo, error) {
	req, err := web.NewHTTPRequest(ipfs.Request)
	if err != nil {
		return nil, err
	}

	req.URL.Path = "/api/v0/stats/repo"
	req.Method = http.MethodPost

	var stats ipfsRepo
	if err := ipfs.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (ipfs *IPFS) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := ipfs.httpClient.Do(req)
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
