// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import "encoding/xml"

type serverStatusResponse struct {
	XMLName xml.Name `xml:"status"`

	JVM struct {
		Memory struct {
			Used  int64 `stm:"used"` // calculated manually
			Free  int64 `xml:"free,attr" stm:"free"`
			Total int64 `xml:"total,attr" stm:"total"`
			Max   int64 `xml:"max,attr"`
		} `xml:"memory" stm:"memory"`

		MemoryPools []struct {
			STMKey string

			Name           string `xml:"name,attr"`
			Type           string `xml:"type,attr"`
			UsageInit      int64  `xml:"usageInit,attr"`
			UsageCommitted int64  `xml:"usageCommitted,attr" stm:"commited"`
			UsageMax       int64  `xml:"usageMax,attr" stm:"max"`
			UsageUsed      int64  `xml:"usageUsed,attr" stm:"used"`
		} `xml:"memorypool" stm:"memorypool"`
	} `xml:"jvm" stm:"jvm"`

	Connectors []struct {
		STMKey string

		Name string `xml:"name,attr"`

		ThreadInfo struct {
			MaxThreads         int64 `xml:"maxThreads,attr"`
			CurrentThreadCount int64 `xml:"currentThreadCount,attr" stm:"count"`
			CurrentThreadsBusy int64 `xml:"currentThreadsBusy,attr" stm:"busy"`
			CurrentThreadsIdle int64 `stm:"idle"` // calculated manually
		} `xml:"threadInfo" stm:"thread_info"`

		RequestInfo struct {
			MaxTime        int64 `xml:"maxTime,attr"`
			ProcessingTime int64 `xml:"processingTime,attr" stm:"processing_time"`
			RequestCount   int64 `xml:"requestCount,attr" stm:"request_count"`
			ErrorCount     int64 `xml:"errorCount,attr" stm:"error_count"`
			BytesReceived  int64 `xml:"bytesReceived,attr" stm:"bytes_received"`
			BytesSent      int64 `xml:"bytesSent,attr" stm:"bytes_sent"`
		} `xml:"requestInfo" stm:"request_info"`
	} `xml:"connector" stm:"connector"`
}
