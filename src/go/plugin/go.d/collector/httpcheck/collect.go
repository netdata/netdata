// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type reqErrCode int

const (
	codeTimeout reqErrCode = iota
	codeRedirect
	codeNoConnection
)

func (c *Collector) collect() (map[string]int64, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, fmt.Errorf("error on creating HTTP requests to %s : %v", c.RequestConfig.URL, err)
	}

	if c.CookieFile != "" {
		if err := c.readCookieFile(); err != nil {
			return nil, fmt.Errorf("error on reading cookie file '%s': %v", c.CookieFile, err)
		}
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	dur := time.Since(start)

	defer web.CloseBody(resp)

	var mx metrics

	if c.isError(err, resp) {
		c.Debug(err)
		c.collectErrResponse(&mx, err)
	} else {
		mx.ResponseTime = durationToMs(dur)
		c.collectOKResponse(&mx, resp)
	}

	if c.metrics.Status != mx.Status {
		mx.InState = c.UpdateEvery
	} else {
		mx.InState = c.metrics.InState + c.UpdateEvery
	}
	c.metrics = mx

	return stm.ToMap(mx), nil
}

func (c *Collector) isError(err error, resp *http.Response) bool {
	return err != nil && !(errors.Is(err, web.ErrRedirectAttempted) && c.acceptedStatuses[resp.StatusCode])
}

func (c *Collector) collectErrResponse(mx *metrics, err error) {
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

func (c *Collector) collectOKResponse(mx *metrics, resp *http.Response) {
	c.Debugf("endpoint '%s' returned %d (%s) HTTP status code", c.URL, resp.StatusCode, resp.Status)

	if !c.acceptedStatuses[resp.StatusCode] {
		mx.Status.BadStatusCode = true
		return
	}

	bs, err := io.ReadAll(resp.Body)
	// golang net/http closes body on redirect
	if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "read on closed response body") {
		c.Warningf("error on reading body : %v", err)
		mx.Status.BadContent = true
		return
	}

	mx.ResponseLength = len(bs)

	if c.reResponse != nil {
		matched := c.reResponse.Match(bs)

		if logger.Level.Enabled(slog.LevelDebug) {
			c.Debugf("response validation: pattern=%s, matched=%v, bodySize=%d", c.reResponse, matched, len(bs))
			if len(bs) <= 1024 {
				c.Debugf("response body: %s", string(bs))
			} else {
				c.Debugf("response body (first 1024 bytes): %s...", string(bs[:1024]))
			}
		}

		if !matched {
			mx.Status.BadContent = true
			return
		}
	}

	if ok := c.checkHeader(resp); !ok {
		mx.Status.BadHeader = true
		return
	}

	mx.Status.Success = true
}

func (c *Collector) checkHeader(resp *http.Response) bool {
	for _, m := range c.headerMatch {
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
			c.Debugf("header match: bad header: exlude '%v' key '%s' value '%s'", m.exclude, m.key, value)
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

func (c *Collector) readCookieFile() error {
	if c.CookieFile == "" {
		return nil
	}

	fi, err := os.Stat(c.CookieFile)
	if err != nil {
		return err
	}

	if c.cookieFileModTime.Equal(fi.ModTime()) {
		c.Debugf("cookie file '%s' modification time has not changed, using previously read data", c.CookieFile)
		return nil
	}

	c.Debugf("reading cookie file '%s'", c.CookieFile)

	jar, err := loadCookieJar(c.CookieFile)
	if err != nil {
		return err
	}

	c.httpClient.Jar = jar
	c.cookieFileModTime = fi.ModTime()

	return nil
}

func durationToMs(duration time.Duration) int {
	return int(duration) / (int(time.Millisecond) / int(time.Nanosecond))
}
