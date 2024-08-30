// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	bytesIn               = "bytes_in"
	bytesOut              = "bytes_out"
	connTotal             = "conn_total"
	reqTotal              = "req_total"
	http2xx               = "http_2xx"
	http3xx               = "http_3xx"
	http4xx               = "http_4xx"
	http5xx               = "http_5xx"
	httpOtherStatus       = "http_other_status"
	rt                    = "rt"
	upsReq                = "ups_req"
	upsRT                 = "ups_rt"
	upsTries              = "ups_tries"
	http200               = "http_200"
	http206               = "http_206"
	http302               = "http_302"
	http304               = "http_304"
	http403               = "http_403"
	http404               = "http_404"
	http416               = "http_416"
	http499               = "http_499"
	http500               = "http_500"
	http502               = "http_502"
	http503               = "http_503"
	http504               = "http_504"
	http508               = "http_508"
	httpOtherDetailStatus = "http_other_detail_status"
	httpUps4xx            = "http_ups_4xx"
	httpUps5xx            = "http_ups_5xx"
)

var defaultLineFormat = []string{
	bytesIn,
	bytesOut,
	connTotal,
	reqTotal,
	http2xx,
	http3xx,
	http4xx,
	http5xx,
	httpOtherStatus,
	rt,
	upsReq,
	upsRT,
	upsTries,
	http200,
	http206,
	http302,
	http304,
	http403,
	http404,
	http416,
	http499,
	http500,
	http502,
	http503,
	http504,
	http508,
	httpOtherDetailStatus,
	httpUps4xx,
	httpUps5xx,
}

func newAPIClient(client *http.Client, request web.Request) *apiClient {
	return &apiClient{httpClient: client, request: request}
}

type apiClient struct {
	httpClient *http.Client
	request    web.Request
}

func (a apiClient) getStatus() (*tengineStatus, error) {
	req, err := web.NewHTTPRequest(a.request)
	if err != nil {
		return nil, fmt.Errorf("error on creating request : %v", err)
	}

	resp, err := a.doRequestOK(req)
	defer closeBody(resp)
	if err != nil {
		return nil, err
	}

	status, err := parseStatus(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error on parsing response : %v", err)
	}

	return status, nil
}

func (a apiClient) doRequestOK(req *http.Request) (*http.Response, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error on request : %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned HTTP code %d", req.URL, resp.StatusCode)
	}
	return resp, nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func parseStatus(r io.Reader) (*tengineStatus, error) {
	var status tengineStatus

	s := bufio.NewScanner(r)
	for s.Scan() {
		m, err := parseStatusLine(s.Text(), defaultLineFormat)
		if err != nil {
			return nil, err
		}
		status = append(status, *m)
	}

	return &status, nil
}

func parseStatusLine(line string, lineFormat []string) (*metric, error) {
	parts := strings.Split(line, ",")

	// NOTE: only default line format is supported
	// TODO: custom line format?
	// www.example.com,127.0.0.1:80,162,6242,1,1,1,0,0,0,0,10,1,10,1....
	i := findFirstInt(parts)
	if i == -1 {
		return nil, fmt.Errorf("invalid line : %s", line)
	}
	if len(parts[i:]) != len(lineFormat) {
		return nil, fmt.Errorf("invalid line length, got %d, expected %d, line : %s",
			len(parts[i:]), len(lineFormat), line)
	}

	// skip "$host,$server_addr:$server_port"
	parts = parts[i:]

	var m metric
	for i, key := range lineFormat {
		value := mustParseInt(parts[i])
		switch key {
		default:
			return nil, fmt.Errorf("unknown line format key: %s", key)
		case bytesIn:
			m.BytesIn = value
		case bytesOut:
			m.BytesOut = value
		case connTotal:
			m.ConnTotal = value
		case reqTotal:
			m.ReqTotal = value
		case http2xx:
			m.HTTP2xx = value
		case http3xx:
			m.HTTP3xx = value
		case http4xx:
			m.HTTP4xx = value
		case http5xx:
			m.HTTP5xx = value
		case httpOtherStatus:
			m.HTTPOtherStatus = value
		case rt:
			m.RT = value
		case upsReq:
			m.UpsReq = value
		case upsRT:
			m.UpsRT = value
		case upsTries:
			m.UpsTries = value
		case http200:
			m.HTTP200 = value
		case http206:
			m.HTTP206 = value
		case http302:
			m.HTTP302 = value
		case http304:
			m.HTTP304 = value
		case http403:
			m.HTTP403 = value
		case http404:
			m.HTTP404 = value
		case http416:
			m.HTTP416 = value
		case http499:
			m.HTTP499 = value
		case http500:
			m.HTTP500 = value
		case http502:
			m.HTTP502 = value
		case http503:
			m.HTTP503 = value
		case http504:
			m.HTTP504 = value
		case http508:
			m.HTTP508 = value
		case httpOtherDetailStatus:
			m.HTTPOtherDetailStatus = value
		case httpUps4xx:
			m.HTTPUps4xx = value
		case httpUps5xx:
			m.HTTPUps5xx = value
		}
	}
	return &m, nil
}

func findFirstInt(s []string) int {
	for i, v := range s {
		_, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		return i
	}
	return -1
}

func mustParseInt(value string) *int64 {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		panic(err)
	}

	return &v
}
