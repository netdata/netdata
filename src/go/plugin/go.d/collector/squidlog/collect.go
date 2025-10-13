// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"io"
	"runtime"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
)

func (c *Collector) logPanicStackIfAny() {
	err := recover()
	if err == nil {
		return
	}
	c.Errorf("[ERROR] %s\n", err)
	for depth := 0; ; depth++ {
		_, file, line, ok := runtime.Caller(depth)
		if !ok {
			break
		}
		c.Errorf("======> %d: %v:%d", depth, file, line)
	}
	panic(err)
}

func (c *Collector) collect() (map[string]int64, error) {
	defer c.logPanicStackIfAny()
	c.mx.reset()

	var mx map[string]int64

	n, err := c.collectLogLines()

	if n > 0 || err == nil {
		mx = stm.ToMap(c.mx)
	}
	return mx, err
}

func (c *Collector) collectLogLines() (int, error) {
	var n int
	for {
		c.line.reset()
		err := c.parser.ReadLine(c.line)
		if err != nil {
			if err == io.EOF {
				return n, nil
			}
			if !logs.IsParseError(err) {
				return n, err
			}
			n++
			c.collectUnmatched()
			continue
		}
		n++
		if c.line.empty() {
			c.collectUnmatched()
		} else {
			c.collectLogLine()
		}
	}
}

func (c *Collector) collectLogLine() {
	c.mx.Requests.Inc()
	c.collectRespTime()
	c.collectClientAddress()
	c.collectCacheCode()
	c.collectHTTPCode()
	c.collectRespSize()
	c.collectReqMethod()
	c.collectHierCode()
	c.collectServerAddress()
	c.collectMimeType()
}

func (c *Collector) collectUnmatched() {
	c.mx.Requests.Inc()
	c.mx.Unmatched.Inc()
}

func (c *Collector) collectRespTime() {
	if !c.line.hasRespTime() {
		return
	}
	c.mx.RespTime.Observe(float64(c.line.respTime))
}

func (c *Collector) collectClientAddress() {
	if !c.line.hasClientAddress() {
		return
	}
	c.mx.UniqueClients.Insert(c.line.clientAddr)
}

func (c *Collector) collectCacheCode() {
	if !c.line.hasCacheCode() {
		return
	}

	cntr, ok := c.mx.CacheCode.GetP(c.line.cacheCode)
	if !ok {
		c.addDimToCacheCodeChart(c.line.cacheCode)
	}
	cntr.Inc()

	tags := strings.Split(c.line.cacheCode, "_")
	for _, tag := range tags {
		c.collectCacheCodeTag(tag)
	}
}

func (c *Collector) collectHTTPCode() {
	if !c.line.hasHTTPCode() {
		return
	}

	code := c.line.httpCode
	switch {
	case code >= 100 && code < 300, code == 0, code == 304, code == 401, code == 429:
		c.mx.ReqSuccess.Inc()
	case code >= 300 && code < 400:
		c.mx.ReqRedirect.Inc()
	case code >= 400 && code < 500:
		c.mx.ReqBad.Inc()
	case code >= 500 && code <= 603:
		c.mx.ReqError.Inc()
	}

	switch code / 100 {
	case 0:
		c.mx.HTTPResp0xx.Inc()
	case 1:
		c.mx.HTTPResp1xx.Inc()
	case 2:
		c.mx.HTTPResp2xx.Inc()
	case 3:
		c.mx.HTTPResp3xx.Inc()
	case 4:
		c.mx.HTTPResp4xx.Inc()
	case 5:
		c.mx.HTTPResp5xx.Inc()
	case 6:
		c.mx.HTTPResp6xx.Inc()
	}

	codeStr := strconv.Itoa(code)
	cntr, ok := c.mx.HTTPRespCode.GetP(codeStr)
	if !ok {
		c.addDimToHTTPRespCodesChart(codeStr)
	}
	cntr.Inc()
}

func (c *Collector) collectRespSize() {
	if !c.line.hasRespSize() {
		return
	}
	c.mx.BytesSent.Add(float64(c.line.respSize))
}

func (c *Collector) collectReqMethod() {
	if !c.line.hasReqMethod() {
		return
	}
	cntr, ok := c.mx.ReqMethod.GetP(c.line.reqMethod)
	if !ok {
		c.addDimToReqMethodChart(c.line.reqMethod)
	}
	cntr.Inc()
}

func (c *Collector) collectHierCode() {
	if !c.line.hasHierCode() {
		return
	}
	cntr, ok := c.mx.HierCode.GetP(c.line.hierCode)
	if !ok {
		c.addDimToHierCodeChart(c.line.hierCode)
	}
	cntr.Inc()
}

func (c *Collector) collectServerAddress() {
	if !c.line.hasServerAddress() {
		return
	}
	cntr, ok := c.mx.Server.GetP(c.line.serverAddr)
	if !ok {
		c.addDimToServerAddressChart(c.line.serverAddr)
	}
	cntr.Inc()
}

func (c *Collector) collectMimeType() {
	if !c.line.hasMimeType() {
		return
	}
	cntr, ok := c.mx.MimeType.GetP(c.line.mimeType)
	if !ok {
		c.addDimToMimeTypeChart(c.line.mimeType)
	}
	cntr.Inc()
}

func (c *Collector) collectCacheCodeTag(tag string) {
	// https://wiki.squid-cache.org/SquidFaq/SquidLogs#Squid_result_codes
	switch tag {
	default:
	case "TCP", "UDP", "NONE":
		cntr, ok := c.mx.CacheCodeTransportTag.GetP(tag)
		if !ok {
			c.addDimToCacheCodeTransportTagChart(tag)
		}
		cntr.Inc()
	case "CF", "CLIENT", "IMS", "ASYNC", "SWAPFAIL", "REFRESH", "SHARED", "REPLY":
		cntr, ok := c.mx.CacheCodeHandlingTag.GetP(tag)
		if !ok {
			c.addDimToCacheCodeHandlingTagChart(tag)
		}
		cntr.Inc()
	case "NEGATIVE", "STALE", "OFFLINE", "INVALID", "FAIL", "MODIFIED", "UNMODIFIED", "REDIRECT":
		cntr, ok := c.mx.CacheCodeObjectTag.GetP(tag)
		if !ok {
			c.addDimToCacheCodeObjectTagChart(tag)
		}
		cntr.Inc()
	case "HIT", "MEM", "MISS", "DENIED", "NOFETCH", "TUNNEL":
		cntr, ok := c.mx.CacheCodeLoadSourceTag.GetP(tag)
		if !ok {
			c.addDimToCacheCodeLoadSourceTagChart(tag)
		}
		cntr.Inc()
	case "ABORTED", "TIMEOUT", "IGNORED":
		cntr, ok := c.mx.CacheCodeErrorTag.GetP(tag)
		if !ok {
			c.addDimToCacheCodeErrorTagChart(tag)
		}
		cntr.Inc()
	}
}

func (c *Collector) addDimToCacheCodeChart(code string) {
	chartID := cacheCodeChart.ID
	dimID := pxCacheCode + code
	c.addDimToChart(chartID, dimID, code)
}

func (c *Collector) addDimToCacheCodeTransportTagChart(tag string) {
	chartID := cacheCodeTransportTagChart.ID
	dimID := pxTransportTag + tag
	c.addDimToChart(chartID, dimID, tag)
}

func (c *Collector) addDimToCacheCodeHandlingTagChart(tag string) {
	chartID := cacheCodeHandlingTagChart.ID
	dimID := pxHandlingTag + tag
	c.addDimToChart(chartID, dimID, tag)
}

func (c *Collector) addDimToCacheCodeObjectTagChart(tag string) {
	chartID := cacheCodeObjectTagChart.ID
	dimID := pxObjectTag + tag
	c.addDimToChart(chartID, dimID, tag)
}

func (c *Collector) addDimToCacheCodeLoadSourceTagChart(tag string) {
	chartID := cacheCodeLoadSourceTagChart.ID
	dimID := pxSourceTag + tag
	c.addDimToChart(chartID, dimID, tag)
}

func (c *Collector) addDimToCacheCodeErrorTagChart(tag string) {
	chartID := cacheCodeErrorTagChart.ID
	dimID := pxErrorTag + tag
	c.addDimToChart(chartID, dimID, tag)
}

func (c *Collector) addDimToHTTPRespCodesChart(tag string) {
	chartID := httpRespCodesChart.ID
	dimID := pxHTTPCode + tag
	c.addDimToChart(chartID, dimID, tag)
}

func (c *Collector) addDimToReqMethodChart(method string) {
	chartID := reqMethodChart.ID
	dimID := pxReqMethod + method
	c.addDimToChart(chartID, dimID, method)
}

func (c *Collector) addDimToHierCodeChart(code string) {
	chartID := hierCodeChart.ID
	dimID := pxHierCode + code
	dimName := code[5:] // remove "HIER_"
	c.addDimToChart(chartID, dimID, dimName)
}

func (c *Collector) addDimToServerAddressChart(address string) {
	chartID := serverAddrChart.ID
	dimID := pxSrvAddr + address
	c.addDimToChartOrCreateIfNotExist(chartID, dimID, address)
}

func (c *Collector) addDimToMimeTypeChart(mimeType string) {
	chartID := mimeTypeChart.ID
	dimID := pxMimeType + mimeType
	c.addDimToChartOrCreateIfNotExist(chartID, dimID, mimeType)
}

func (c *Collector) addDimToChart(chartID, dimID, dimName string) {
	chart := c.Charts().Get(chartID)
	if chart == nil {
		c.Warningf("add '%s' dim: couldn't find '%s' chart in charts", dimID, chartID)
		return
	}

	dim := &Dim{ID: dimID, Name: dimName, Algo: module.Incremental}

	if err := chart.AddDim(dim); err != nil {
		c.Warningf("add '%s' dim: %v", dimID, err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToChartOrCreateIfNotExist(chartID, dimID, dimName string) {
	if c.Charts().Has(chartID) {
		c.addDimToChart(chartID, dimID, dimName)
		return
	}

	chart := newChartByID(chartID)
	if chart == nil {
		c.Warningf("add '%s' dim: couldn't create '%s' chart", dimID, chartID)
		return
	}
	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
		return
	}
	c.addDimToChart(chartID, dimID, dimName)
}

func newChartByID(chartID string) *Chart {
	switch chartID {
	case serverAddrChart.ID:
		return serverAddrChart.Copy()
	case mimeTypeChart.ID:
		return mimeTypeChart.Copy()
	}
	return nil
}
