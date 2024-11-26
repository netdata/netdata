// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"io"
	"runtime"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (s *SquidLog) logPanicStackIfAny() {
	err := recover()
	if err == nil {
		return
	}
	s.Errorf("[ERROR] %s\n", err)
	for depth := 0; ; depth++ {
		_, file, line, ok := runtime.Caller(depth)
		if !ok {
			break
		}
		s.Errorf("======> %d: %v:%d", depth, file, line)
	}
	panic(err)
}

func (s *SquidLog) collect() (map[string]int64, error) {
	defer s.logPanicStackIfAny()
	s.mx.reset()

	var mx map[string]int64

	n, err := s.collectLogLines()

	if n > 0 || err == nil {
		mx = stm.ToMap(s.mx)
	}
	return mx, err
}

func (s *SquidLog) collectLogLines() (int, error) {
	var n int
	for {
		s.line.reset()
		err := s.parser.ReadLine(s.line)
		if err != nil {
			if err == io.EOF {
				return n, nil
			}
			if !logs.IsParseError(err) {
				return n, err
			}
			n++
			s.collectUnmatched()
			continue
		}
		n++
		if s.line.empty() {
			s.collectUnmatched()
		} else {
			s.collectLogLine()
		}
	}
}

func (s *SquidLog) collectLogLine() {
	s.mx.Requests.Inc()
	s.collectRespTime()
	s.collectClientAddress()
	s.collectCacheCode()
	s.collectHTTPCode()
	s.collectRespSize()
	s.collectReqMethod()
	s.collectHierCode()
	s.collectServerAddress()
	s.collectMimeType()
}

func (s *SquidLog) collectUnmatched() {
	s.mx.Requests.Inc()
	s.mx.Unmatched.Inc()
}

func (s *SquidLog) collectRespTime() {
	if !s.line.hasRespTime() {
		return
	}
	s.mx.RespTime.Observe(float64(s.line.respTime))
}

func (s *SquidLog) collectClientAddress() {
	if !s.line.hasClientAddress() {
		return
	}
	s.mx.UniqueClients.Insert(s.line.clientAddr)
}

func (s *SquidLog) collectCacheCode() {
	if !s.line.hasCacheCode() {
		return
	}

	c, ok := s.mx.CacheCode.GetP(s.line.cacheCode)
	if !ok {
		s.addDimToCacheCodeChart(s.line.cacheCode)
	}
	c.Inc()

	tags := strings.Split(s.line.cacheCode, "_")
	for _, tag := range tags {
		s.collectCacheCodeTag(tag)
	}
}

func (s *SquidLog) collectHTTPCode() {
	if !s.line.hasHTTPCode() {
		return
	}

	code := s.line.httpCode
	switch {
	case code >= 100 && code < 300, code == 0, code == 304, code == 401:
		s.mx.ReqSuccess.Inc()
	case code >= 300 && code < 400:
		s.mx.ReqRedirect.Inc()
	case code >= 400 && code < 500:
		s.mx.ReqBad.Inc()
	case code >= 500 && code <= 603:
		s.mx.ReqError.Inc()
	}

	switch code / 100 {
	case 0:
		s.mx.HTTPResp0xx.Inc()
	case 1:
		s.mx.HTTPResp1xx.Inc()
	case 2:
		s.mx.HTTPResp2xx.Inc()
	case 3:
		s.mx.HTTPResp3xx.Inc()
	case 4:
		s.mx.HTTPResp4xx.Inc()
	case 5:
		s.mx.HTTPResp5xx.Inc()
	case 6:
		s.mx.HTTPResp6xx.Inc()
	}

	codeStr := strconv.Itoa(code)
	c, ok := s.mx.HTTPRespCode.GetP(codeStr)
	if !ok {
		s.addDimToHTTPRespCodesChart(codeStr)
	}
	c.Inc()
}

func (s *SquidLog) collectRespSize() {
	if !s.line.hasRespSize() {
		return
	}
	s.mx.BytesSent.Add(float64(s.line.respSize))
}

func (s *SquidLog) collectReqMethod() {
	if !s.line.hasReqMethod() {
		return
	}
	c, ok := s.mx.ReqMethod.GetP(s.line.reqMethod)
	if !ok {
		s.addDimToReqMethodChart(s.line.reqMethod)
	}
	c.Inc()
}

func (s *SquidLog) collectHierCode() {
	if !s.line.hasHierCode() {
		return
	}
	c, ok := s.mx.HierCode.GetP(s.line.hierCode)
	if !ok {
		s.addDimToHierCodeChart(s.line.hierCode)
	}
	c.Inc()
}

func (s *SquidLog) collectServerAddress() {
	if !s.line.hasServerAddress() {
		return
	}
	c, ok := s.mx.Server.GetP(s.line.serverAddr)
	if !ok {
		s.addDimToServerAddressChart(s.line.serverAddr)
	}
	c.Inc()
}

func (s *SquidLog) collectMimeType() {
	if !s.line.hasMimeType() {
		return
	}
	c, ok := s.mx.MimeType.GetP(s.line.mimeType)
	if !ok {
		s.addDimToMimeTypeChart(s.line.mimeType)
	}
	c.Inc()
}

func (s *SquidLog) collectCacheCodeTag(tag string) {
	// https://wiki.squid-cache.org/SquidFaq/SquidLogs#Squid_result_codes
	switch tag {
	default:
	case "TCP", "UDP", "NONE":
		c, ok := s.mx.CacheCodeTransportTag.GetP(tag)
		if !ok {
			s.addDimToCacheCodeTransportTagChart(tag)
		}
		c.Inc()
	case "CF", "CLIENT", "IMS", "ASYNC", "SWAPFAIL", "REFRESH", "SHARED", "REPLY":
		c, ok := s.mx.CacheCodeHandlingTag.GetP(tag)
		if !ok {
			s.addDimToCacheCodeHandlingTagChart(tag)
		}
		c.Inc()
	case "NEGATIVE", "STALE", "OFFLINE", "INVALID", "FAIL", "MODIFIED", "UNMODIFIED", "REDIRECT":
		c, ok := s.mx.CacheCodeObjectTag.GetP(tag)
		if !ok {
			s.addDimToCacheCodeObjectTagChart(tag)
		}
		c.Inc()
	case "HIT", "MEM", "MISS", "DENIED", "NOFETCH", "TUNNEL":
		c, ok := s.mx.CacheCodeLoadSourceTag.GetP(tag)
		if !ok {
			s.addDimToCacheCodeLoadSourceTagChart(tag)
		}
		c.Inc()
	case "ABORTED", "TIMEOUT", "IGNORED":
		c, ok := s.mx.CacheCodeErrorTag.GetP(tag)
		if !ok {
			s.addDimToCacheCodeErrorTagChart(tag)
		}
		c.Inc()
	}
}

func (s *SquidLog) addDimToCacheCodeChart(code string) {
	chartID := cacheCodeChart.ID
	dimID := pxCacheCode + code
	s.addDimToChart(chartID, dimID, code)
}

func (s *SquidLog) addDimToCacheCodeTransportTagChart(tag string) {
	chartID := cacheCodeTransportTagChart.ID
	dimID := pxTransportTag + tag
	s.addDimToChart(chartID, dimID, tag)
}

func (s *SquidLog) addDimToCacheCodeHandlingTagChart(tag string) {
	chartID := cacheCodeHandlingTagChart.ID
	dimID := pxHandlingTag + tag
	s.addDimToChart(chartID, dimID, tag)
}

func (s *SquidLog) addDimToCacheCodeObjectTagChart(tag string) {
	chartID := cacheCodeObjectTagChart.ID
	dimID := pxObjectTag + tag
	s.addDimToChart(chartID, dimID, tag)
}

func (s *SquidLog) addDimToCacheCodeLoadSourceTagChart(tag string) {
	chartID := cacheCodeLoadSourceTagChart.ID
	dimID := pxSourceTag + tag
	s.addDimToChart(chartID, dimID, tag)
}

func (s *SquidLog) addDimToCacheCodeErrorTagChart(tag string) {
	chartID := cacheCodeErrorTagChart.ID
	dimID := pxErrorTag + tag
	s.addDimToChart(chartID, dimID, tag)
}

func (s *SquidLog) addDimToHTTPRespCodesChart(tag string) {
	chartID := httpRespCodesChart.ID
	dimID := pxHTTPCode + tag
	s.addDimToChart(chartID, dimID, tag)
}

func (s *SquidLog) addDimToReqMethodChart(method string) {
	chartID := reqMethodChart.ID
	dimID := pxReqMethod + method
	s.addDimToChart(chartID, dimID, method)
}

func (s *SquidLog) addDimToHierCodeChart(code string) {
	chartID := hierCodeChart.ID
	dimID := pxHierCode + code
	dimName := code[5:] // remove "HIER_"
	s.addDimToChart(chartID, dimID, dimName)
}

func (s *SquidLog) addDimToServerAddressChart(address string) {
	chartID := serverAddrChart.ID
	dimID := pxSrvAddr + address
	s.addDimToChartOrCreateIfNotExist(chartID, dimID, address)
}

func (s *SquidLog) addDimToMimeTypeChart(mimeType string) {
	chartID := mimeTypeChart.ID
	dimID := pxMimeType + mimeType
	s.addDimToChartOrCreateIfNotExist(chartID, dimID, mimeType)
}

func (s *SquidLog) addDimToChart(chartID, dimID, dimName string) {
	chart := s.Charts().Get(chartID)
	if chart == nil {
		s.Warningf("add '%s' dim: couldn't find '%s' chart in charts", dimID, chartID)
		return
	}

	dim := &Dim{ID: dimID, Name: dimName, Algo: module.Incremental}

	if err := chart.AddDim(dim); err != nil {
		s.Warningf("add '%s' dim: %v", dimID, err)
		return
	}
	chart.MarkNotCreated()
}

func (s *SquidLog) addDimToChartOrCreateIfNotExist(chartID, dimID, dimName string) {
	if s.Charts().Has(chartID) {
		s.addDimToChart(chartID, dimID, dimName)
		return
	}

	chart := newChartByID(chartID)
	if chart == nil {
		s.Warningf("add '%s' dim: couldn't create '%s' chart", dimID, chartID)
		return
	}
	if err := s.Charts().Add(chart); err != nil {
		s.Warning(err)
		return
	}
	s.addDimToChart(chartID, dimID, dimName)
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
