// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

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
