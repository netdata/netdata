// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type (
	ipfsStatsBw struct {
		TotalIn  int64    `json:"TotalIn"`
		TotalOut int64    `json:"TotalOut"`
		RateIn   *float64 `json:"RateIn"`
		RateOut  *float64 `json:"RateOut"`
	}
	ipfsStatsRepo struct {
		RepoSize   int64 `json:"RepoSize"`
		StorageMax int64 `json:"StorageMax"`
		NumObjects int64 `json:"NumObjects"`
	}
	ipfsSwarmPeers struct {
		Peers []any `json:"Peers"`
	}
	ipfsPinsLs struct {
		Keys map[string]struct {
			Type string `json:"type"`
		} `json:"Keys"`
	}
)

const (
	urlPathStatsBandwidth = "/api/v0/stats/bw"    // https://docs.ipfs.tech/reference/kubo/rpc/#api-v0-stats-bw
	urlPathStatsRepo      = "/api/v0/stats/repo"  // https://docs.ipfs.tech/reference/kubo/rpc/#api-v0-stats-repo
	urlPathSwarmPeers     = "/api/v0/swarm/peers" // https://docs.ipfs.tech/reference/kubo/rpc/#api-v0-swarm-peers
	urlPathPinLs          = "/api/v0/pin/ls"      // https://docs.ipfs.tech/reference/kubo/rpc/#api-v0-pin-ls
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectStatsBandwidth(mx); err != nil {
		return nil, err
	}
	if err := c.collectSwarmPeers(mx); err != nil {
		return nil, err
	}
	if c.QueryRepoApi {
		// https://github.com/netdata/netdata/pull/9687
		// TODO: collect by default with "size-only"
		// https://github.com/ipfs/kubo/issues/7528#issuecomment-657398332
		if err := c.collectStatsRepo(mx); err != nil {
			return nil, err
		}
	}
	if c.QueryPinApi {
		if err := c.collectPinLs(mx); err != nil {
			return nil, err
		}
	}

	return mx, nil
}

func (c *Collector) collectStatsBandwidth(mx map[string]int64) error {
	stats, err := c.queryStatsBandwidth()
	if err != nil {
		return err
	}

	mx["in"] = stats.TotalIn
	mx["out"] = stats.TotalOut

	return nil
}

func (c *Collector) collectSwarmPeers(mx map[string]int64) error {
	stats, err := c.querySwarmPeers()
	if err != nil {
		return err
	}

	mx["peers"] = int64(len(stats.Peers))

	return nil
}

func (c *Collector) collectStatsRepo(mx map[string]int64) error {
	stats, err := c.queryStatsRepo()
	if err != nil {
		return err
	}

	mx["used_percent"] = 0
	if stats.StorageMax > 0 {
		mx["used_percent"] = stats.RepoSize * 100 / stats.StorageMax
	}
	mx["size"] = stats.RepoSize
	mx["objects"] = stats.NumObjects

	return nil
}

func (c *Collector) collectPinLs(mx map[string]int64) error {
	stats, err := c.queryPinLs()
	if err != nil {
		return err
	}

	var n int64
	for _, v := range stats.Keys {
		if v.Type == "recursive" {
			n++
		}
	}

	mx["pinned"] = int64(len(stats.Keys))
	mx["recursive_pins"] = n

	return nil
}

func (c *Collector) queryStatsBandwidth() (*ipfsStatsBw, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathStatsBandwidth)
	if err != nil {
		return nil, err
	}

	var stats ipfsStatsBw
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	if stats.RateIn == nil || stats.RateOut == nil {
		return nil, fmt.Errorf("unexpected response: not ipfs data")
	}

	return &stats, nil
}

func (c *Collector) querySwarmPeers() (*ipfsSwarmPeers, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathSwarmPeers)
	if err != nil {
		return nil, err
	}

	var stats ipfsSwarmPeers
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (c *Collector) queryStatsRepo() (*ipfsStatsRepo, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathStatsRepo)
	if err != nil {
		return nil, err
	}

	var stats ipfsStatsRepo
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (c *Collector) queryPinLs() (*ipfsPinsLs, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathPinLs)
	if err != nil {
		return nil, err
	}

	var stats ipfsPinsLs
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}
