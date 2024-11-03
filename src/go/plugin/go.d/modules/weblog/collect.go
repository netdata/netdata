package weblog

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (w *WebLog) logPanicStackIfAny() {
	err := recover()
	if err == nil {
		return
	}
	w.Errorf("[ERROR] %s\n", err)
	for depth := 0; ; depth++ {
		_, file, line, ok := runtime.Caller(depth)
		if !ok {
			break
		}
		w.Errorf("======> %d: %v:%d", depth, file, line)
	}
	panic(err)
}

func (w *WebLog) collect() (map[string]int64, error) {
	defer w.logPanicStackIfAny()
	w.mx.reset()

	var mx map[string]int64

	n, err := w.collectLogLines()

	if n > 0 || err == nil {
		mx = stm.ToMap(w.mx)
	}
	return mx, err
}

func (w *WebLog) collectLogLines() (int, error) {
	logOnce := true
	var n int
	for {
		w.line.reset()
		err := w.parser.ReadLine(w.line)
		if err != nil {
			if err == io.EOF {
				return n, nil
			}
			if !logs.IsParseError(err) {
				return n, err
			}
			n++
			if logOnce {
				w.Infof("unmatched line: %v (parser: %s)", err, w.parser.Info())
				logOnce = false
			}
			w.collectUnmatched()
			continue
		}
		n++
		if w.line.empty() {
			w.collectUnmatched()
		} else {
			w.collectLogLine()
		}
	}
}

func (w *WebLog) collectLogLine() {
	// https://github.com/netdata/netdata/issues/17716
	if w.line.hasReqProcTime() && w.line.respCode == http.StatusSwitchingProtocols {
		w.line.reqProcTime = emptyNumber
	}
	w.mx.Requests.Inc()
	w.collectVhost()
	w.collectPort()
	w.collectReqScheme()
	w.collectReqClient()
	w.collectReqMethod()
	w.collectReqURL()
	w.collectReqProto()
	w.collectRespCode()
	w.collectReqSize()
	w.collectRespSize()
	w.collectReqProcTime()
	w.collectUpsRespTime()
	w.collectSSLProto()
	w.collectSSLCipherSuite()
	w.collectCustomFields()
}

func (w *WebLog) collectUnmatched() {
	w.mx.Requests.Inc()
	w.mx.ReqUnmatched.Inc()
}

func (w *WebLog) collectVhost() {
	if !w.line.hasVhost() {
		return
	}
	c, ok := w.mx.ReqVhost.GetP(w.line.vhost)
	if !ok {
		w.addDimToVhostChart(w.line.vhost)
	}
	c.Inc()
	if w.line.hasReqSize() {
		w.mx.ReqVhostBytesReceived.Add(float64(w.line.reqSize))
	}
	if w.line.hasRespSize() {
		w.mx.ReqVhostBytesSent.Add(float64(w.line.respSize))
	}
}

func (w *WebLog) collectPort() {
	if !w.line.hasPort() {
		return
	}
	c, ok := w.mx.ReqPort.GetP(w.line.port)
	if !ok {
		w.addDimToPortChart(w.line.port)
	}
	c.Inc()
}

func (w *WebLog) collectReqClient() {
	if !w.line.hasReqClient() {
		return
	}
	if strings.ContainsRune(w.line.reqClient, ':') {
		w.mx.ReqIPv6.Inc()
		w.mx.UniqueIPv6.Insert(w.line.reqClient)
		return
	}
	// NOTE: count hostname as IPv4 address
	w.mx.ReqIPv4.Inc()
	w.mx.UniqueIPv4.Insert(w.line.reqClient)
}

func (w *WebLog) collectReqScheme() {
	if !w.line.hasReqScheme() {
		return
	}
	if w.line.reqScheme == "https" {
		w.mx.ReqHTTPSScheme.Inc()
	} else {
		w.mx.ReqHTTPScheme.Inc()
	}
}

func (w *WebLog) collectReqMethod() {
	if !w.line.hasReqMethod() {
		return
	}
	c, ok := w.mx.ReqMethod.GetP(w.line.reqMethod)
	if !ok {
		w.addDimToReqMethodChart(w.line.reqMethod)
	}
	c.Inc()
}

func (w *WebLog) collectReqURL() {
	if !w.line.hasReqURL() {
		return
	}
	for _, p := range w.urlPatterns {
		if !p.MatchString(w.line.reqURL) {
			continue
		}
		c, _ := w.mx.ReqURLPattern.GetP(p.name)
		c.Inc()

		w.collectURLPatternStats(p.name)
		return
	}
}

func (w *WebLog) collectReqProto() {
	if !w.line.hasReqProto() {
		return
	}
	c, ok := w.mx.ReqVersion.GetP(w.line.reqProto)
	if !ok {
		w.addDimToReqVersionChart(w.line.reqProto)
	}
	c.Inc()
}

func (w *WebLog) collectRespCode() {
	if !w.line.hasRespCode() {
		return
	}

	code := w.line.respCode
	switch {
	case code >= 100 && code < 300, code == 304, code == 401:
		w.mx.ReqSuccess.Inc()
	case code >= 300 && code < 400:
		w.mx.ReqRedirect.Inc()
	case code >= 400 && code < 500:
		w.mx.ReqBad.Inc()
	case code >= 500 && code < 600:
		w.mx.ReqError.Inc()
	}

	switch code / 100 {
	case 1:
		w.mx.Resp1xx.Inc()
	case 2:
		w.mx.Resp2xx.Inc()
	case 3:
		w.mx.Resp3xx.Inc()
	case 4:
		w.mx.Resp4xx.Inc()
	case 5:
		w.mx.Resp5xx.Inc()
	}

	codeStr := strconv.Itoa(code)
	c, ok := w.mx.RespCode.GetP(codeStr)
	if !ok {
		w.addDimToRespCodesChart(codeStr)
	}
	c.Inc()
}

func (w *WebLog) collectReqSize() {
	if !w.line.hasReqSize() {
		return
	}
	w.mx.BytesReceived.Add(float64(w.line.reqSize))
}

func (w *WebLog) collectRespSize() {
	if !w.line.hasRespSize() {
		return
	}
	w.mx.BytesSent.Add(float64(w.line.respSize))
}

func (w *WebLog) collectReqProcTime() {
	if !w.line.hasReqProcTime() {
		return
	}
	w.mx.ReqProcTime.Observe(w.line.reqProcTime)
	if w.mx.ReqProcTimeHist == nil {
		return
	}
	w.mx.ReqProcTimeHist.Observe(w.line.reqProcTime)
}

func (w *WebLog) collectUpsRespTime() {
	if !w.line.hasUpsRespTime() {
		return
	}
	w.mx.UpsRespTime.Observe(w.line.upsRespTime)
	if w.mx.UpsRespTimeHist == nil {
		return
	}
	w.mx.UpsRespTimeHist.Observe(w.line.upsRespTime)
}

func (w *WebLog) collectSSLProto() {
	if !w.line.hasSSLProto() {
		return
	}
	c, ok := w.mx.ReqSSLProto.GetP(w.line.sslProto)
	if !ok {
		w.addDimToSSLProtoChart(w.line.sslProto)
	}
	c.Inc()
}

func (w *WebLog) collectSSLCipherSuite() {
	if !w.line.hasSSLCipherSuite() {
		return
	}
	c, ok := w.mx.ReqSSLCipherSuite.GetP(w.line.sslCipherSuite)
	if !ok {
		w.addDimToSSLCipherSuiteChart(w.line.sslCipherSuite)
	}
	c.Inc()
}

func (w *WebLog) collectURLPatternStats(name string) {
	v, ok := w.mx.URLPatternStats[name]
	if !ok {
		return
	}
	if w.line.hasRespCode() {
		status := strconv.Itoa(w.line.respCode)
		c, ok := v.RespCode.GetP(status)
		if !ok {
			w.addDimToURLPatternRespCodesChart(name, status)
		}
		c.Inc()
	}

	if w.line.hasReqMethod() {
		c, ok := v.ReqMethod.GetP(w.line.reqMethod)
		if !ok {
			w.addDimToURLPatternReqMethodsChart(name, w.line.reqMethod)
		}
		c.Inc()
	}

	if w.line.hasReqSize() {
		v.BytesReceived.Add(float64(w.line.reqSize))
	}

	if w.line.hasRespSize() {
		v.BytesSent.Add(float64(w.line.respSize))
	}
	if w.line.hasReqProcTime() {
		v.ReqProcTime.Observe(w.line.reqProcTime)
	}
}

func (w *WebLog) collectCustomFields() {
	if !w.line.hasCustomFields() {
		return
	}

	for _, cv := range w.line.custom.values {
		_, _ = cv.name, cv.value

		if patterns, ok := w.customFields[cv.name]; ok {
			for _, pattern := range patterns {
				if !pattern.MatchString(cv.value) {
					continue
				}
				v, ok := w.mx.ReqCustomField[cv.name]
				if !ok {
					break
				}
				c, _ := v.GetP(pattern.name)
				c.Inc()
				break
			}
		} else if histogram, ok := w.customTimeFields[cv.name]; ok {
			v, ok := w.mx.ReqCustomTimeField[cv.name]
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
		} else if w.customNumericFields[cv.name] {
			m, ok := w.mx.ReqCustomNumericField[cv.name]
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

func (w *WebLog) addDimToVhostChart(vhost string) {
	chart := w.Charts().Get(reqByVhost.ID)
	if chart == nil {
		w.Warningf("add dimension: no '%s' chart", reqByVhost.ID)
		return
	}
	dim := &Dim{
		ID:   "req_vhost_" + vhost,
		Name: vhost,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) addDimToPortChart(port string) {
	chart := w.Charts().Get(reqByPort.ID)
	if chart == nil {
		w.Warningf("add dimension: no '%s' chart", reqByPort.ID)
		return
	}
	dim := &Dim{
		ID:   "req_port_" + port,
		Name: port,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) addDimToReqMethodChart(method string) {
	chart := w.Charts().Get(reqByMethod.ID)
	if chart == nil {
		w.Warningf("add dimension: no '%s' chart", reqByMethod.ID)
		return
	}
	dim := &Dim{
		ID:   "req_method_" + method,
		Name: method,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) addDimToReqVersionChart(version string) {
	chart := w.Charts().Get(reqByVersion.ID)
	if chart == nil {
		w.Warningf("add dimension: no '%s' chart", reqByVersion.ID)
		return
	}
	dim := &Dim{
		ID:   "req_version_" + version,
		Name: version,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) addDimToSSLProtoChart(proto string) {
	chart := w.Charts().Get(reqBySSLProto.ID)
	if chart == nil {
		chart = reqBySSLProto.Copy()
		if err := w.Charts().Add(chart); err != nil {
			w.Warning(err)
			return
		}
	}
	dim := &Dim{
		ID:   "req_ssl_proto_" + proto,
		Name: proto,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) addDimToSSLCipherSuiteChart(cipher string) {
	chart := w.Charts().Get(reqBySSLCipherSuite.ID)
	if chart == nil {
		chart = reqBySSLCipherSuite.Copy()
		if err := w.Charts().Add(chart); err != nil {
			w.Warning(err)
			return
		}
	}
	dim := &Dim{
		ID:   "req_ssl_cipher_suite_" + cipher,
		Name: cipher,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) addDimToRespCodesChart(code string) {
	chart := w.findRespCodesChart(code)
	if chart == nil {
		w.Warning("add dimension: cant find resp codes chart")
		return
	}
	dim := &Dim{
		ID:   "resp_code_" + code,
		Name: code,
		Algo: module.Incremental,
	}
	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) addDimToURLPatternRespCodesChart(name, code string) {
	id := fmt.Sprintf(urlPatternRespCodes.ID, name)
	chart := w.Charts().Get(id)
	if chart == nil {
		w.Warningf("add dimension: no '%s' chart", id)
		return
	}
	dim := &Dim{
		ID:   fmt.Sprintf("url_ptn_%s_resp_code_%s", name, code),
		Name: code,
		Algo: module.Incremental,
	}

	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) addDimToURLPatternReqMethodsChart(name, method string) {
	id := fmt.Sprintf(urlPatternReqMethods.ID, name)
	chart := w.Charts().Get(id)
	if chart == nil {
		w.Warningf("add dimension: no '%s' chart", id)
		return
	}
	dim := &Dim{
		ID:   fmt.Sprintf("url_ptn_%s_req_method_%s", name, method),
		Name: method,
		Algo: module.Incremental,
	}

	if err := chart.AddDim(dim); err != nil {
		w.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (w *WebLog) findRespCodesChart(code string) *Chart {
	if !w.GroupRespCodes {
		return w.Charts().Get(respCodes.ID)
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
	return w.Charts().Get(id)
}
