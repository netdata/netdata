// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"fmt"
	"io"
	"net/http"
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
	logOnce := true
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
			if logOnce {
				c.Infof("unmatched line: %v (parser: %s)", err, c.parser.Info())
				logOnce = false
			}
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
	// https://github.com/netdata/netdata/issues/17716
	if c.line.hasReqProcTime() && c.line.respCode == http.StatusSwitchingProtocols {
		c.line.reqProcTime = emptyNumber
	}
	c.mx.Requests.Inc()
	c.collectVhost()
	c.collectPort()
	c.collectReqScheme()
	c.collectReqClient()
	c.collectReqMethod()
	c.collectReqURL()
	c.collectReqProto()
	c.collectRespCode()
	c.collectReqSize()
	c.collectRespSize()
	c.collectReqProcTime()
	c.collectUpsRespTime()
	c.collectSSLProto()
	c.collectSSLCipherSuite()
	c.collectCustomFields()
}

func (c *Collector) collectUnmatched() {
	c.mx.Requests.Inc()
	c.mx.ReqUnmatched.Inc()
}

func (c *Collector) collectVhost() {
	if !c.line.hasVhost() {
		return
	}
	cntr, ok := c.mx.ReqVhost.GetP(c.line.vhost)
	if !ok {
		c.addDimToVhostChart(c.line.vhost)
	}
	cntr.Inc()
}

func (c *Collector) collectPort() {
	if !c.line.hasPort() {
		return
	}
	cntr, ok := c.mx.ReqPort.GetP(c.line.port)
	if !ok {
		c.addDimToPortChart(c.line.port)
	}
	cntr.Inc()
}

func (c *Collector) collectReqClient() {
	if !c.line.hasReqClient() {
		return
	}
	if strings.ContainsRune(c.line.reqClient, ':') {
		c.mx.ReqIPv6.Inc()
		c.mx.UniqueIPv6.Insert(c.line.reqClient)
		return
	}
	// NOTE: count hostname as IPv4 address
	c.mx.ReqIPv4.Inc()
	c.mx.UniqueIPv4.Insert(c.line.reqClient)
}

func (c *Collector) collectReqScheme() {
	if !c.line.hasReqScheme() {
		return
	}
	if c.line.reqScheme == "https" {
		c.mx.ReqHTTPSScheme.Inc()
	} else {
		c.mx.ReqHTTPScheme.Inc()
	}
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

func (c *Collector) collectReqURL() {
	if !c.line.hasReqURL() {
		return
	}
	for _, p := range c.urlPatterns {
		if !p.MatchString(c.line.reqURL) {
			continue
		}
		cntr, _ := c.mx.ReqURLPattern.GetP(p.name)
		cntr.Inc()

		c.collectURLPatternStats(p.name)
		return
	}
}

func (c *Collector) collectReqProto() {
	if !c.line.hasReqProto() {
		return
	}
	cntr, ok := c.mx.ReqVersion.GetP(c.line.reqProto)
	if !ok {
		c.addDimToReqVersionChart(c.line.reqProto)
	}
	cntr.Inc()
}

func (c *Collector) collectRespCode() {
	if !c.line.hasRespCode() {
		return
	}

	code := c.line.respCode
	switch {
	case code >= 100 && code < 300, code == 304, code == 401, code == 429:
		c.mx.ReqSuccess.Inc()
	case code >= 300 && code < 400:
		c.mx.ReqRedirect.Inc()
	case code >= 400 && code < 500:
		c.mx.ReqBad.Inc()
	case code >= 500 && code < 600:
		c.mx.ReqError.Inc()
	}

	switch code / 100 {
	case 1:
		c.mx.Resp1xx.Inc()
	case 2:
		c.mx.Resp2xx.Inc()
	case 3:
		c.mx.Resp3xx.Inc()
	case 4:
		c.mx.Resp4xx.Inc()
	case 5:
		c.mx.Resp5xx.Inc()
	}

	codeStr := strconv.Itoa(code)
	cntr, ok := c.mx.RespCode.GetP(codeStr)
	if !ok {
		c.addDimToRespCodesChart(codeStr)
	}
	cntr.Inc()
}

func (c *Collector) collectReqSize() {
	if !c.line.hasReqSize() {
		return
	}
	c.mx.BytesReceived.Add(float64(c.line.reqSize))
}

func (c *Collector) collectRespSize() {
	if !c.line.hasRespSize() {
		return
	}
	c.mx.BytesSent.Add(float64(c.line.respSize))
}

func (c *Collector) collectReqProcTime() {
	if !c.line.hasReqProcTime() {
		return
	}
	c.mx.ReqProcTime.Observe(c.line.reqProcTime)
	if c.mx.ReqProcTimeHist == nil {
		return
	}
	c.mx.ReqProcTimeHist.Observe(c.line.reqProcTime)
}

func (c *Collector) collectUpsRespTime() {
	if !c.line.hasUpsRespTime() {
		return
	}
	c.mx.UpsRespTime.Observe(c.line.upsRespTime)
	if c.mx.UpsRespTimeHist == nil {
		return
	}
	c.mx.UpsRespTimeHist.Observe(c.line.upsRespTime)
}

func (c *Collector) collectSSLProto() {
	if !c.line.hasSSLProto() {
		return
	}
	cntr, ok := c.mx.ReqSSLProto.GetP(c.line.sslProto)
	if !ok {
		c.addDimToSSLProtoChart(c.line.sslProto)
	}
	cntr.Inc()
}

func (c *Collector) collectSSLCipherSuite() {
	if !c.line.hasSSLCipherSuite() {
		return
	}
	cntr, ok := c.mx.ReqSSLCipherSuite.GetP(c.line.sslCipherSuite)
	if !ok {
		c.addDimToSSLCipherSuiteChart(c.line.sslCipherSuite)
	}
	cntr.Inc()
}

func (c *Collector) collectURLPatternStats(name string) {
	v, ok := c.mx.URLPatternStats[name]
	if !ok {
		return
	}
	if c.line.hasRespCode() {
		status := strconv.Itoa(c.line.respCode)
		cntr, ok := v.RespCode.GetP(status)
		if !ok {
			c.addDimToURLPatternRespCodesChart(name, status)
		}
		cntr.Inc()
	}

	if c.line.hasReqMethod() {
		cntr, ok := v.ReqMethod.GetP(c.line.reqMethod)
		if !ok {
			c.addDimToURLPatternReqMethodsChart(name, c.line.reqMethod)
		}
		cntr.Inc()
	}

	if c.line.hasReqSize() {
		v.BytesReceived.Add(float64(c.line.reqSize))
	}

	if c.line.hasRespSize() {
		v.BytesSent.Add(float64(c.line.respSize))
	}
	if c.line.hasReqProcTime() {
		v.ReqProcTime.Observe(c.line.reqProcTime)
	}
}

func (c *Collector) collectCustomFields() {
	if !c.line.hasCustomFields() {
		return
	}

	for _, cv := range c.line.custom.values {
		_, _ = cv.name, cv.value

		if patterns, ok := c.customFields[cv.name]; ok {
			for _, pattern := range patterns {
				if !pattern.MatchString(cv.value) {
					continue
				}
				v, ok := c.mx.ReqCustomField[cv.name]
				if !ok {
					break
				}
				c, _ := v.GetP(pattern.name)
				c.Inc()
				break
			}
		} else if histogram, ok := c.customTimeFields[cv.name]; ok {
			v, ok := c.mx.ReqCustomTimeField[cv.name]
			if !ok {
				continue
			}
			ctf, err := strconv.ParseFloat(cv.value, 64)
			if err != nil || !isTimeValid(ctf) {
				continue
			}
			v.Time.Observe(ctf)
			if histogram != nil {
				v.TimeHist.Observe(ctf * timeMultiplier(cv.value))
			}
		} else if c.customNumericFields[cv.name] {
			m, ok := c.mx.ReqCustomNumericField[cv.name]
			if !ok {
				continue
			}
			v, err := strconv.ParseFloat(cv.value, 64)
			if err != nil {
				continue
			}
			v *= float64(m.multiplier)
			m.Summary.Observe(v)
		}
	}
}

func (c *Collector) addDimToVhostChart(vhost string) {
	chart := c.Charts().Get(reqByVhost.ID)
	if chart == nil {
		c.Warningf("add dimension: no '%s' chart", reqByVhost.ID)
		return
	}
	dim := &Dim{
		ID:   "req_vhost_" + vhost,
		Name: vhost,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToPortChart(port string) {
	chart := c.Charts().Get(reqByPort.ID)
	if chart == nil {
		c.Warningf("add dimension: no '%s' chart", reqByPort.ID)
		return
	}
	dim := &Dim{
		ID:   "req_port_" + port,
		Name: port,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToReqMethodChart(method string) {
	chart := c.Charts().Get(reqByMethod.ID)
	if chart == nil {
		c.Warningf("add dimension: no '%s' chart", reqByMethod.ID)
		return
	}
	dim := &Dim{
		ID:   "req_method_" + method,
		Name: method,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToReqVersionChart(version string) {
	chart := c.Charts().Get(reqByVersion.ID)
	if chart == nil {
		c.Warningf("add dimension: no '%s' chart", reqByVersion.ID)
		return
	}
	dim := &Dim{
		ID:   "req_version_" + version,
		Name: version,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToSSLProtoChart(proto string) {
	chart := c.Charts().Get(reqBySSLProto.ID)
	if chart == nil {
		chart = reqBySSLProto.Copy()
		if err := c.Charts().Add(chart); err != nil {
			c.Warning(err)
			return
		}
	}
	dim := &Dim{
		ID:   "req_ssl_proto_" + proto,
		Name: proto,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToSSLCipherSuiteChart(cipher string) {
	chart := c.Charts().Get(reqBySSLCipherSuite.ID)
	if chart == nil {
		chart = reqBySSLCipherSuite.Copy()
		if err := c.Charts().Add(chart); err != nil {
			c.Warning(err)
			return
		}
	}
	dim := &Dim{
		ID:   "req_ssl_cipher_suite_" + cipher,
		Name: cipher,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToRespCodesChart(code string) {
	chart := c.findRespCodesChart(code)
	if chart == nil {
		c.Warning("add dimension: cant find resp codes chart")
		return
	}
	dim := &Dim{
		ID:   "resp_code_" + code,
		Name: code,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToURLPatternRespCodesChart(name, code string) {
	id := fmt.Sprintf(urlPatternRespCodes.ID, name)
	chart := c.Charts().Get(id)
	if chart == nil {
		c.Warningf("add dimension: no '%s' chart", id)
		return
	}
	dim := &Dim{
		ID:   fmt.Sprintf("url_ptn_%s_resp_code_%s", name, code),
		Name: code,
		Algo: module.Incremental,
	}

	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addDimToURLPatternReqMethodsChart(name, method string) {
	id := fmt.Sprintf(urlPatternReqMethods.ID, name)
	chart := c.Charts().Get(id)
	if chart == nil {
		c.Warningf("add dimension: no '%s' chart", id)
		return
	}
	dim := &Dim{
		ID:   fmt.Sprintf("url_ptn_%s_req_method_%s", name, method),
		Name: method,
		Algo: module.Incremental,
	}

	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) findRespCodesChart(code string) *Chart {
	if !c.GroupRespCodes {
		return c.Charts().Get(respCodes.ID)
	}

	var id string
	switch class := code[:1]; class {
	case "1":
		id = respCodes1xx.ID
	case "2":
		id = respCodes2xx.ID
	case "3":
		id = respCodes3xx.ID
	case "4":
		id = respCodes4xx.ID
	case "5":
		id = respCodes5xx.ID
	default:
		return nil
	}
	return c.Charts().Get(id)
}
