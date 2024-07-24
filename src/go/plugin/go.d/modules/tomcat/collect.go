// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathServerStats = "/manager/status?XML=true"
)

func (tc *Tomcat) collect() (map[string]int64, error) {
	mx := make(map[string]int64)
	response, err := tc.queryServerStatus()
	if err != nil {
		return nil, err
	}

	// JVM Memory
	mx["jvm.free"] = int64(response.Jvm.Memory.Free)
	mx["jvm.total"] = int64(response.Jvm.Memory.Total)
	mx["jvm.max"] = int64(response.Jvm.Memory.Max)

	for _, pool := range response.Jvm.MemoryPools {
		name := pool.Name
		switch name {
		case "G1 Eden Space":
			mx["eden_used"] = int64(pool.UsageUsed)
			mx["eden_committed"] = int64(pool.UsageCommitted)
			mx["eden_max"] = int64(pool.UsageMax)
		case "G1 Survivor Space":
			mx["survivor_used"] = int64(pool.UsageUsed)
			mx["survivor_committed"] = int64(pool.UsageCommitted)
			mx["survivor_max"] = int64(pool.UsageMax)
		case "G1 Old Gen":
			mx["tenured_used"] = int64(pool.UsageUsed)
			mx["tenured_committed"] = int64(pool.UsageCommitted)
			mx["tenured_max"] = int64(pool.UsageMax)
		case "CodeHeap 'non-nmethods'", "CodeHeap 'non-profiled nmethods'", "CodeHeap 'profiled nmethods'":
			mx["code_cache_used"] = int64(pool.UsageUsed)
			mx["code_cache_committed"] = int64(pool.UsageCommitted)
			mx["code_cache_max"] = int64(pool.UsageMax)
		case "Compressed Class Space":
			mx["compressed_used"] = int64(pool.UsageUsed)
			mx["compressed_committed"] = int64(pool.UsageCommitted)
			mx["compressed_max"] = int64(pool.UsageMax)
		case "Metaspace":
			mx["metaspace_used"] = int64(pool.UsageUsed)
			mx["metaspace_committed"] = int64(pool.UsageCommitted)
			mx["metaspace_max"] = int64(pool.UsageMax)
		}
	}

	if response.Connector.Name != "" {
		mx["request_count"] = int64(response.Connector.RequestInfo.RequestCount)
		mx["error_count"] = int64(response.Connector.RequestInfo.ErrorCount)
		mx["bytes_sent"] = int64(response.Connector.RequestInfo.BytesSent)
		mx["bytes_received"] = int64(response.Connector.RequestInfo.BytesReceived)
		mx["processing_time"] = int64(response.Connector.RequestInfo.ProcessingTime)
		mx["current_thread_count"] = int64(response.Connector.ThreadInfo.CurrentThreadCount)
		mx["busy_thread_count"] = int64(response.Connector.ThreadInfo.CurrentThreadsBusy)
		// connectorName := connector.Name
	}

	return mx, nil
}

type Status struct {
	XMLName   xml.Name  `xml:"status"`
	Jvm       Jvm       `xml:"jvm"`
	Connector Connector `xml:"connector"`
}

type Jvm struct {
	XMLName     xml.Name     `xml:"jvm"`
	Memory      Memory       `xml:"memory"`
	MemoryPools []MemoryPool `xml:"memorypool"`
}

type Memory struct {
	XMLName xml.Name `xml:"memory"`
	Free    int      `xml:"free,attr"`
	Total   int      `xml:"total,attr"`
	Max     int      `xml:"max,attr"`
}

type MemoryPool struct {
	XMLName        xml.Name `xml:"memorypool"`
	Name           string   `xml:"name,attr"`
	Type           string   `xml:"type,attr"`
	UsageInit      int      `xml:"usageInit,attr"`
	UsageCommitted int      `xml:"usageCommitted,attr"`
	UsageMax       int      `xml:"usageMax,attr"`
	UsageUsed      int      `xml:"usageUsed,attr"`
}

type Connector struct {
	XMLName     xml.Name    `xml:"connector"`
	Name        string      `xml:"name,attr"`
	ThreadInfo  ThreadInfo  `xml:"threadInfo"`
	RequestInfo RequestInfo `xml:"requestInfo"`
}

type ThreadInfo struct {
	XMLName            xml.Name `xml:"threadInfo"`
	MaxThreads         int      `xml:"maxThreads,attr"`
	CurrentThreadCount int      `xml:"currentThreadCount,attr"`
	CurrentThreadsBusy int      `xml:"currentThreadsBusy,attr"`
}

type RequestInfo struct {
	XMLName        xml.Name `xml:"requestInfo"`
	MaxTime        int      `xml:"maxTime,attr"`
	ProcessingTime int      `xml:"processingTime,attr"`
	RequestCount   int      `xml:"requestCount,attr"`
	ErrorCount     int      `xml:"errorCount,attr"`
	BytesReceived  int      `xml:"bytesReceived,attr"`
	BytesSent      int      `xml:"bytesSent,attr"`
}

func (tc *Tomcat) queryServerStatus() (*Status, error) {
	req, err := web.NewHTTPRequestWithPath(tc.Request, urlPathServerStats)
	if err != nil {
		return nil, err
	}

	var stats Status

	if err := tc.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (tc *Tomcat) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body from '%s': %v", req.URL, err)
	}

	bodyStr := string(body)
	err = xml.Unmarshal([]byte(bodyStr), in)
	if err != nil {
		return fmt.Errorf("error decoding XML response from '%s': %v", req.URL, err)
	}
	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
