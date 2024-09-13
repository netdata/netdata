// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

/*
http://tengine.taobao.org/document/http_reqstat.html

bytes_in total number of bytes received from client
bytes_out total number of bytes sent to client
conn_total total number of accepted connections
req_total total number of processed requests
http_2xx total number of 2xx requests
http_3xx total number of 3xx requests
http_4xx total number of 4xx requests
http_5xx total number of 5xx requests
http_other_status total number of other requests
rt accumulation or rt
ups_req total number of requests calling for upstream
ups_rt accumulation or upstream rt
ups_tries total number of times calling for upstream
http_200 total number of 200 requests
http_206 total number of 206 requests
http_302 total number of 302 requests
http_304 total number of 304 requests
http_403 total number of 403 requests
http_404 total number of 404 requests
http_416 total number of 416 requests
http_499 total number of 499 requests
http_500 total number of 500 requests
http_502 total number of 502 requests
http_503 total number of 503 requests
http_504 total number of 504 requests
http_508 total number of 508 requests
http_other_detail_status total number of requests of other status codes
http_ups_4xx total number of requests of upstream 4xx
http_ups_5xx total number of requests of upstream 5xx
*/

type (
	tengineStatus []metric

	metric struct {
		Host                  string
		ServerAddress         string
		BytesIn               *int64 `stm:"bytes_in"`
		BytesOut              *int64 `stm:"bytes_out"`
		ConnTotal             *int64 `stm:"conn_total"`
		ReqTotal              *int64 `stm:"req_total"`
		HTTP2xx               *int64 `stm:"http_2xx"`
		HTTP3xx               *int64 `stm:"http_3xx"`
		HTTP4xx               *int64 `stm:"http_4xx"`
		HTTP5xx               *int64 `stm:"http_5xx"`
		HTTPOtherStatus       *int64 `stm:"http_other_status"`
		RT                    *int64 `stm:"rt"`
		UpsReq                *int64 `stm:"ups_req"`
		UpsRT                 *int64 `stm:"ups_rt"`
		UpsTries              *int64 `stm:"ups_tries"`
		HTTP200               *int64 `stm:"http_200"`
		HTTP206               *int64 `stm:"http_206"`
		HTTP302               *int64 `stm:"http_302"`
		HTTP304               *int64 `stm:"http_304"`
		HTTP403               *int64 `stm:"http_403"`
		HTTP404               *int64 `stm:"http_404"`
		HTTP416               *int64 `stm:"http_416"`
		HTTP499               *int64 `stm:"http_499"`
		HTTP500               *int64 `stm:"http_500"`
		HTTP502               *int64 `stm:"http_502"`
		HTTP503               *int64 `stm:"http_503"`
		HTTP504               *int64 `stm:"http_504"`
		HTTP508               *int64 `stm:"http_508"`
		HTTPOtherDetailStatus *int64 `stm:"http_other_detail_status"`
		HTTPUps4xx            *int64 `stm:"http_ups_4xx"`
		HTTPUps5xx            *int64 `stm:"http_ups_5xx"`
	}
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
