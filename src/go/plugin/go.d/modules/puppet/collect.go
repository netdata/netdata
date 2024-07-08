// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type PuppetStatsService struct {
	StatusService ServiceData `json:"status-service"`
}

type ServiceData struct {
	Status Status `json:"status"`
}

type Status struct {
	Experimental Experimental `json:"experimental"`
}

type Experimental struct {
	JVMMetrics *JVMMetrics `json:"jvm-metrics,omitempty"`
}
type JVMMetrics struct {
	CPUUsage        float64                  `json:"cpu-usage"`
	UpTimeMS        int64                    `json:"up-time-ms"`
	MemoryPools     map[string]MemoryPool    `json:"memory-pools"`
	GCCPUUsage      float64                  `json:"gc-cpu-usage"`
	Threading       Threading                `json:"threading"`
	HeapMemory      MemoryUsage              `json:"heap-memory"`
	GCStats         map[string]GCStats       `json:"gc-stats"`
	StartTimeMS     int64                    `json:"start-time-ms"`
	FileDescriptors FileDescriptors          `json:"file-descriptors"`
	NonHeapMemory   MemoryUsage              `json:"non-heap-memory"`
	NIOBufferPools  map[string]NIOBufferPool `json:"nio-buffer-pools"`
}

type MemoryPool struct {
	Type  string      `json:"type"`
	Usage MemoryUsage `json:"usage"`
}

type MemoryUsage struct {
	Committed int64 `json:"committed"`
	Init      int64 `json:"init"`
	Max       int64 `json:"max"`
	Used      int64 `json:"used"`
}

type Threading struct {
	ThreadCount     int `json:"thread-count"`
	PeakThreadCount int `json:"peak-thread-count"`
}

type GCStats struct {
	Count       int         `json:"count"`
	TotalTimeMS int64       `json:"total-time-ms"`
	LastGCInfo  *LastGCInfo `json:"last-gc-info,omitempty"`
}

type LastGCInfo struct {
	DurationMS int64 `json:"duration-ms"`
}

type FileDescriptors struct {
	Used int `json:"used"`
	Max  int `json:"max"`
}

type NIOBufferPool struct {
	Count         int64 `json:"count"`
	MemoryUsed    int64 `json:"memory-used"`
	TotalCapacity int64 `json:"total-capacity"`
}

const (
	urlPathStatsService = "/status/v1/services?level=debug" //https://puppet.com/docs/puppet/8/server/status-api/v1/services
)

func (ppt *Puppet) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := ppt.collectStatsService(mx); err != nil {
		return nil, err
	}
	// if err := ppt.collectSwarmPeers(mx); err != nil {
	// 	return nil, err
	// }
	// if ppt.QueryRepoApi {
	// 	// https://github.com/netdata/netdata/pull/9687
	// 	// TODO: collect by default with "size-only"
	// 	// https://github.com/puppet/kubo/issues/7528#issuecomment-657398332
	// 	if err := ppt.collectStatsRepo(mx); err != nil {
	// 		return nil, err
	// 	}
	// }
	// if ppt.QueryPinApi {
	// 	if err := ppt.collectPinLs(mx); err != nil {
	// 		return nil, err
	// 	}
	// }

	return mx, nil
}

func (ppt *Puppet) collectStatsService(mx map[string]int64) error {
	stats, err := ppt.queryStatsService()
	if err != nil {
		return err
	}

	mx["jvm_heap_committed"] = stats.StatusService.Status.Experimental.JVMMetrics.HeapMemory.Committed
	mx["jvm_heap_used"] = stats.StatusService.Status.Experimental.JVMMetrics.HeapMemory.Used
	mx["jvm_nonheap_committed"] = stats.StatusService.Status.Experimental.JVMMetrics.NonHeapMemory.Committed
	mx["jvm_nonheap_used"] = stats.StatusService.Status.Experimental.JVMMetrics.NonHeapMemory.Used
	mx["cpu_time"] = int64(stats.StatusService.Status.Experimental.JVMMetrics.CPUUsage * 1000)
	mx["gc_time"] = int64(stats.StatusService.Status.Experimental.JVMMetrics.GCCPUUsage * 1000)
	mx["fd_used"] = int64(stats.StatusService.Status.Experimental.JVMMetrics.FileDescriptors.Used * 1000)

	//vars
	mx["jvm_heap_max"] = stats.StatusService.Status.Experimental.JVMMetrics.HeapMemory.Max
	mx["jvm_heap_init"] = stats.StatusService.Status.Experimental.JVMMetrics.HeapMemory.Init
	mx["jvm_nonheap_max"] = stats.StatusService.Status.Experimental.JVMMetrics.NonHeapMemory.Max
	mx["jvm_nonheap_init"] = stats.StatusService.Status.Experimental.JVMMetrics.NonHeapMemory.Init
	mx["fd_max"] = int64(stats.StatusService.Status.Experimental.JVMMetrics.FileDescriptors.Max * 1000)

	return nil
}

// func (ppt *Puppet) collectSwarmPeers(mx map[string]int64) error {
// 	stats, err := ppt.querySwarmPeers()
// 	if err != nil {
// 		return err
// 	}

// 	mx["peers"] = int64(len(stats.Peers))

// 	return nil
// }

// func (ppt *Puppet) collectStatsRepo(mx map[string]int64) error {
// 	stats, err := ppt.queryStatsRepo()
// 	if err != nil {
// 		return err
// 	}

// 	mx["used_percent"] = 0
// 	if stats.StorageMax > 0 {
// 		mx["used_percent"] = stats.RepoSize * 100 / stats.StorageMax
// 	}
// 	mx["size"] = stats.RepoSize
// 	mx["objects"] = stats.NumObjects

// 	return nil
// }

// func (ppt *Puppet) collectPinLs(mx map[string]int64) error {
// 	stats, err := ppt.queryPinLs()
// 	if err != nil {
// 		return err
// 	}

// 	var n int64
// 	for _, v := range stats.Keys {
// 		if v.Type == "recursive" {
// 			n++
// 		}
// 	}

// 	mx["pinned"] = int64(len(stats.Keys))
// 	mx["recursive_pins"] = n

// 	return nil
// }

func (ppt *Puppet) queryStatsService() (*PuppetStatsService, error) {
	req, err := web.NewHTTPRequest(ppt.Request)
	if err != nil {
		return nil, err
	}

	req.URL.Path = "/status/v1/services"
	req.URL.RawQuery = "level=debug"

	var stats PuppetStatsService
	if err := ppt.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	if stats.StatusService == (ServiceData{}) {
		return nil, fmt.Errorf("unexpected response: not puppet service status data")
	}

	return &stats, nil
}

func (ppt *Puppet) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := ppt.httpClient.Do(req)
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
