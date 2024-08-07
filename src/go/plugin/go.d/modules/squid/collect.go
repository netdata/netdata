// SPDX-License-Identifier: GPL-3.0-or-later

package squid

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathServerStats = "/squid-internal-mgr/counters" // https://wiki.squid-cache.org/Features/CacheManager/Index#controlling-access-to-the-cache-manager
)

var statsCounters = map[string]bool{
	"client_http.kbytes_in":      true,
	"server.all.errors":          true,
	"server.all.requests":        true,
	"server.all.kbytes_out":      true,
	"server.all.kbytes_in":       true,
	"client_http.errors":         true,
	"client_http.hits":           true,
	"client_http.requests":       true,
	"client_http.hit_kbytes_out": true,
}

func (sq *Squid) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := sq.collectCounters(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (sq *Squid) collectCounters(mx map[string]int64) error {
	rawStats, err := sq.queryCounters()

	if err != nil {
		return err
	}

	if len(*rawStats) == 0 {
		return fmt.Errorf("empty response")
	}

	if !strings.HasPrefix(*rawStats, "sample_time") {
		return fmt.Errorf("unexpected response: not a squid counters response %s", *rawStats)
	}

	lines := strings.Split(*rawStats, "\n")

	if len(lines) < 1 {
		return fmt.Errorf("empty response")
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		parts := strings.SplitN(line, "=", 2)

		if len(line) < 1 {
			sq.Debug("empty line in the response")
			continue
		} else if len(parts) != 2 {
			return fmt.Errorf(line, "not like x = y")
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if !statsCounters[key] {
			continue
		}
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			mx[key] = v
		}
	}

	return nil
}

func (sq *Squid) queryCounters() (*string, error) {
	req, err := web.NewHTTPRequestWithPath(sq.Request, urlPathServerStats)
	if err != nil {
		return nil, err
	}

	var stats string

	if err := sq.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (sq *Squid) doOKDecode(req *http.Request, returnString *string) error {
	resp, err := sq.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	*returnString = string(body)

	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
