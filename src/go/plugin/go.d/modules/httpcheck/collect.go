// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type reqErrCode int

const (
	codeTimeout reqErrCode = iota
	codeRedirect
	codeNoConnection
)

func (hc *HTTPCheck) collect() (map[string]int64, error) {
	req, err := web.NewHTTPRequest(hc.RequestConfig)
	if err != nil {
		return nil, fmt.Errorf("error on creating HTTP requests to %s : %v", hc.RequestConfig.URL, err)
	}

	if hc.CookieFile != "" {
		if err := hc.readCookieFile(); err != nil {
			return nil, fmt.Errorf("error on reading cookie file '%s': %v", hc.CookieFile, err)
		}
	}

	start := time.Now()
	resp, err := hc.httpClient.Do(req)
	dur := time.Since(start)

	defer web.CloseBody(resp)

	var mx metrics

	if hc.isError(err, resp) {
		hc.Debug(err)
		hc.collectErrResponse(&mx, err)
	} else {
		mx.ResponseTime = durationToMs(dur)
		hc.collectOKResponse(&mx, resp)
	}

	if hc.metrics.Status != mx.Status {
		mx.InState = hc.UpdateEvery
	} else {
		mx.InState = hc.metrics.InState + hc.UpdateEvery
	}
	hc.metrics = mx

	return stm.ToMap(mx), nil
}

func (hc *HTTPCheck) isError(err error, resp *http.Response) bool {
	return err != nil && !(errors.Is(err, web.ErrRedirectAttempted) && hc.acceptedStatuses[resp.StatusCode])
}

func (hc *HTTPCheck) collectErrResponse(mx *metrics, err error) {
	switch code := decodeReqError(err); code {
	case codeNoConnection:
		mx.Status.NoConnection = true
	case codeTimeout:
		mx.Status.Timeout = true
	case codeRedirect:
		mx.Status.Redirect = true
	default:
		panic(fmt.Sprintf("unknown request error code : %d", code))
	}
}

func (hc *HTTPCheck) collectOKResponse(mx *metrics, resp *http.Response) {
	hc.Debugf("endpoint '%s' returned %d (%s) HTTP status code", hc.URL, resp.StatusCode, resp.Status)

	if !hc.acceptedStatuses[resp.StatusCode] {
		mx.Status.BadStatusCode = true
		return
	}

	bs, err := io.ReadAll(resp.Body)
	// golang net/http closes body on redirect
	if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "read on closed response body") {
		hc.Warningf("error on reading body : %v", err)
		mx.Status.BadContent = true
		return
	}

	mx.ResponseLength = len(bs)

	if hc.reResponse != nil && !hc.reResponse.Match(bs) {
		mx.Status.BadContent = true
		return
	}

	if ok := hc.checkHeader(resp); !ok {
		mx.Status.BadHeader = true
		return
	}

	mx.Status.Success = true
}

func (hc *HTTPCheck) checkHeader(resp *http.Response) bool {
	for _, m := range hc.headerMatch {
		value := resp.Header.Get(m.key)

		var ok bool
		switch {
		case value == "":
			ok = m.exclude
		case m.valMatcher == nil:
			ok = !m.exclude
		default:
			ok = m.valMatcher.MatchString(value)
		}

		if !ok {
			hc.Debugf("header match: bad header: exlude '%v' key '%s' value '%s'", m.exclude, m.key, value)
			return false
		}
	}

	return true
}

func decodeReqError(err error) reqErrCode {
	if err == nil {
		panic("nil error")
	}

	if errors.Is(err, web.ErrRedirectAttempted) {
		return codeRedirect
	}
	var v net.Error
	if errors.As(err, &v) && v.Timeout() {
		return codeTimeout
	}
	return codeNoConnection
}

func (hc *HTTPCheck) readCookieFile() error {
	if hc.CookieFile == "" {
		return nil
	}

	fi, err := os.Stat(hc.CookieFile)
	if err != nil {
		return err
	}

	if hc.cookieFileModTime.Equal(fi.ModTime()) {
		hc.Debugf("cookie file '%s' modification time has not changed, using previously read data", hc.CookieFile)
		return nil
	}

	hc.Debugf("reading cookie file '%s'", hc.CookieFile)

	jar, err := loadCookieJar(hc.CookieFile)
	if err != nil {
		return err
	}

	hc.httpClient.Jar = jar
	hc.cookieFileModTime = fi.ModTime()

	return nil
}

func durationToMs(duration time.Duration) int {
	return int(duration) / (int(time.Millisecond) / int(time.Nanosecond))
}
