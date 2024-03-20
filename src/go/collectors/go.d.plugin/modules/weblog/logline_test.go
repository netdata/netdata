// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	emptyStr = ""
)

var emptyLogLine = *newEmptyLogLine()

func TestLogLine_Assign(t *testing.T) {
	type subTest struct {
		input    string
		wantLine logLine
		wantErr  error
	}
	type test struct {
		name   string
		fields []string
		cases  []subTest
	}
	tests := []test{
		{
			name: "Vhost",
			fields: []string{
				"host",
				"http_host",
				"v",
			},
			cases: []subTest{
				{input: "1.1.1.1", wantLine: logLine{web: web{vhost: "1.1.1.1"}}},
				{input: "::1", wantLine: logLine{web: web{vhost: "::1"}}},
				{input: "[::1]", wantLine: logLine{web: web{vhost: "::1"}}},
				{input: "1ce:1ce::babe", wantLine: logLine{web: web{vhost: "1ce:1ce::babe"}}},
				{input: "[1ce:1ce::babe]", wantLine: logLine{web: web{vhost: "1ce:1ce::babe"}}},
				{input: "localhost", wantLine: logLine{web: web{vhost: "localhost"}}},
				{input: "debian10.debian", wantLine: logLine{web: web{vhost: "debian10.debian"}}},
				{input: "my_vhost", wantLine: logLine{web: web{vhost: "my_vhost"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
			},
		},
		{
			name: "Server Port",
			fields: []string{
				"server_port",
				"p",
			},
			cases: []subTest{
				{input: "80", wantLine: logLine{web: web{port: "80"}}},
				{input: "8081", wantLine: logLine{web: web{port: "8081"}}},
				{input: "30000", wantLine: logLine{web: web{port: "30000"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadPort},
				{input: "0", wantLine: emptyLogLine, wantErr: errBadPort},
				{input: "50000", wantLine: emptyLogLine, wantErr: errBadPort},
			},
		},
		{
			name: "Vhost With Port",
			fields: []string{
				"host:$server_port",
				"v:%p",
			},
			cases: []subTest{
				{input: "1.1.1.1:80", wantLine: logLine{web: web{vhost: "1.1.1.1", port: "80"}}},
				{input: "::1:80", wantLine: logLine{web: web{vhost: "::1", port: "80"}}},
				{input: "[::1]:80", wantLine: logLine{web: web{vhost: "::1", port: "80"}}},
				{input: "1ce:1ce::babe:80", wantLine: logLine{web: web{vhost: "1ce:1ce::babe", port: "80"}}},
				{input: "debian10.debian:81", wantLine: logLine{web: web{vhost: "debian10.debian", port: "81"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "1.1.1.1", wantLine: emptyLogLine, wantErr: errBadVhostPort},
				{input: "1.1.1.1:", wantLine: emptyLogLine, wantErr: errBadVhostPort},
				{input: "1.1.1.1 80", wantLine: emptyLogLine, wantErr: errBadVhostPort},
				{input: "1.1.1.1:20", wantLine: emptyLogLine, wantErr: errBadVhostPort},
				{input: "1.1.1.1:50000", wantLine: emptyLogLine, wantErr: errBadVhostPort},
			},
		},
		{
			name: "Scheme",
			fields: []string{
				"scheme",
			},
			cases: []subTest{
				{input: "http", wantLine: logLine{web: web{reqScheme: "http"}}},
				{input: "https", wantLine: logLine{web: web{reqScheme: "https"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "HTTP", wantLine: emptyLogLine, wantErr: errBadReqScheme},
				{input: "HTTPS", wantLine: emptyLogLine, wantErr: errBadReqScheme},
			},
		},
		{
			name: "Client",
			fields: []string{
				"remote_addr",
				"a",
				"h",
			},
			cases: []subTest{
				{input: "1.1.1.1", wantLine: logLine{web: web{reqClient: "1.1.1.1"}}},
				{input: "debian10", wantLine: logLine{web: web{reqClient: "debian10"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
			},
		},
		{
			name: "Request",
			fields: []string{
				"request",
				"r",
			},
			cases: []subTest{
				{input: "GET / HTTP/1.0", wantLine: logLine{web: web{reqMethod: "GET", reqURL: "/", reqProto: "1.0"}}},
				{input: "HEAD /ihs.gif HTTP/1.0", wantLine: logLine{web: web{reqMethod: "HEAD", reqURL: "/ihs.gif", reqProto: "1.0"}}},
				{input: "POST /ihs.gif HTTP/1.0", wantLine: logLine{web: web{reqMethod: "POST", reqURL: "/ihs.gif", reqProto: "1.0"}}},
				{input: "PUT /ihs.gif HTTP/1.0", wantLine: logLine{web: web{reqMethod: "PUT", reqURL: "/ihs.gif", reqProto: "1.0"}}},
				{input: "PATCH /ihs.gif HTTP/1.0", wantLine: logLine{web: web{reqMethod: "PATCH", reqURL: "/ihs.gif", reqProto: "1.0"}}},
				{input: "DELETE /ihs.gif HTTP/1.0", wantLine: logLine{web: web{reqMethod: "DELETE", reqURL: "/ihs.gif", reqProto: "1.0"}}},
				{input: "OPTIONS /ihs.gif HTTP/1.0", wantLine: logLine{web: web{reqMethod: "OPTIONS", reqURL: "/ihs.gif", reqProto: "1.0"}}},
				{input: "TRACE /ihs.gif HTTP/1.0", wantLine: logLine{web: web{reqMethod: "TRACE", reqURL: "/ihs.gif", reqProto: "1.0"}}},
				{input: "CONNECT ip.cn:443 HTTP/1.1", wantLine: logLine{web: web{reqMethod: "CONNECT", reqURL: "ip.cn:443", reqProto: "1.1"}}},
				{input: "MKCOL ip.cn:443 HTTP/1.1", wantLine: logLine{web: web{reqMethod: "MKCOL", reqURL: "ip.cn:443", reqProto: "1.1"}}},
				{input: "PROPFIND ip.cn:443 HTTP/1.1", wantLine: logLine{web: web{reqMethod: "PROPFIND", reqURL: "ip.cn:443", reqProto: "1.1"}}},
				{input: "MOVE ip.cn:443 HTTP/1.1", wantLine: logLine{web: web{reqMethod: "MOVE", reqURL: "ip.cn:443", reqProto: "1.1"}}},
				{input: "SEARCH ip.cn:443 HTTP/1.1", wantLine: logLine{web: web{reqMethod: "SEARCH", reqURL: "ip.cn:443", reqProto: "1.1"}}},
				{input: "GET / HTTP/1.1", wantLine: logLine{web: web{reqMethod: "GET", reqURL: "/", reqProto: "1.1"}}},
				{input: "GET / HTTP/2", wantLine: logLine{web: web{reqMethod: "GET", reqURL: "/", reqProto: "2"}}},
				{input: "GET / HTTP/2.0", wantLine: logLine{web: web{reqMethod: "GET", reqURL: "/", reqProto: "2.0"}}},
				{input: "GET /invalid_version http/1.1", wantLine: logLine{web: web{reqMethod: "GET", reqURL: "/invalid_version", reqProto: emptyString}}, wantErr: errBadReqProto},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "GET no_version", wantLine: emptyLogLine, wantErr: errBadRequest},
				{input: "GOT / HTTP/2", wantLine: emptyLogLine, wantErr: errBadReqMethod},
				{input: "get / HTTP/2", wantLine: emptyLogLine, wantErr: errBadReqMethod},
				{input: "x04\x01\x00P$3\xFE\xEA\x00", wantLine: emptyLogLine, wantErr: errBadRequest},
			},
		},
		{
			name: "Request HTTP Method",
			fields: []string{
				"request_method",
				"m",
			},
			cases: []subTest{
				{input: "GET", wantLine: logLine{web: web{reqMethod: "GET"}}},
				{input: "HEAD", wantLine: logLine{web: web{reqMethod: "HEAD"}}},
				{input: "POST", wantLine: logLine{web: web{reqMethod: "POST"}}},
				{input: "PUT", wantLine: logLine{web: web{reqMethod: "PUT"}}},
				{input: "PATCH", wantLine: logLine{web: web{reqMethod: "PATCH"}}},
				{input: "DELETE", wantLine: logLine{web: web{reqMethod: "DELETE"}}},
				{input: "OPTIONS", wantLine: logLine{web: web{reqMethod: "OPTIONS"}}},
				{input: "TRACE", wantLine: logLine{web: web{reqMethod: "TRACE"}}},
				{input: "CONNECT", wantLine: logLine{web: web{reqMethod: "CONNECT"}}},
				{input: "MKCOL", wantLine: logLine{web: web{reqMethod: "MKCOL"}}},
				{input: "PROPFIND", wantLine: logLine{web: web{reqMethod: "PROPFIND"}}},
				{input: "MOVE", wantLine: logLine{web: web{reqMethod: "MOVE"}}},
				{input: "SEARCH", wantLine: logLine{web: web{reqMethod: "SEARCH"}}},
				{input: "PURGE", wantLine: logLine{web: web{reqMethod: "PURGE"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "GET no_version", wantLine: emptyLogLine, wantErr: errBadReqMethod},
				{input: "GOT / HTTP/2", wantLine: emptyLogLine, wantErr: errBadReqMethod},
				{input: "get / HTTP/2", wantLine: emptyLogLine, wantErr: errBadReqMethod},
			},
		},
		{
			name: "Request URL",
			fields: []string{
				"request_uri",
				"U",
			},
			cases: []subTest{
				{input: "/server-status?auto", wantLine: logLine{web: web{reqURL: "/server-status?auto"}}},
				{input: "/default.html", wantLine: logLine{web: web{reqURL: "/default.html"}}},
				{input: "10.0.0.1:3128", wantLine: logLine{web: web{reqURL: "10.0.0.1:3128"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
			},
		},
		{
			name: "Request HTTP Protocol",
			fields: []string{
				"server_protocol",
				"H",
			},
			cases: []subTest{
				{input: "HTTP/1.0", wantLine: logLine{web: web{reqProto: "1.0"}}},
				{input: "HTTP/1.1", wantLine: logLine{web: web{reqProto: "1.1"}}},
				{input: "HTTP/2", wantLine: logLine{web: web{reqProto: "2"}}},
				{input: "HTTP/2.0", wantLine: logLine{web: web{reqProto: "2.0"}}},
				{input: "HTTP/3", wantLine: logLine{web: web{reqProto: "3"}}},
				{input: "HTTP/3.0", wantLine: logLine{web: web{reqProto: "3.0"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "1.1", wantLine: emptyLogLine, wantErr: errBadReqProto},
				{input: "http/1.1", wantLine: emptyLogLine, wantErr: errBadReqProto},
			},
		},
		{
			name: "Response Status Code",
			fields: []string{
				"status",
				"s",
				">s",
			},
			cases: []subTest{
				{input: "100", wantLine: logLine{web: web{respCode: 100}}},
				{input: "200", wantLine: logLine{web: web{respCode: 200}}},
				{input: "300", wantLine: logLine{web: web{respCode: 300}}},
				{input: "400", wantLine: logLine{web: web{respCode: 400}}},
				{input: "500", wantLine: logLine{web: web{respCode: 500}}},
				{input: "600", wantLine: logLine{web: web{respCode: 600}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "99", wantLine: emptyLogLine, wantErr: errBadRespCode},
				{input: "601", wantLine: emptyLogLine, wantErr: errBadRespCode},
				{input: "200 ", wantLine: emptyLogLine, wantErr: errBadRespCode},
				{input: "0.222", wantLine: emptyLogLine, wantErr: errBadRespCode},
				{input: "localhost", wantLine: emptyLogLine, wantErr: errBadRespCode},
			},
		},
		{
			name: "Request Size",
			fields: []string{
				"request_length",
				"I",
			},
			cases: []subTest{
				{input: "15", wantLine: logLine{web: web{reqSize: 15}}},
				{input: "1000000", wantLine: logLine{web: web{reqSize: 1000000}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: logLine{web: web{reqSize: 0}}},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadReqSize},
				{input: "100.222", wantLine: emptyLogLine, wantErr: errBadReqSize},
				{input: "invalid", wantLine: emptyLogLine, wantErr: errBadReqSize},
			},
		},
		{
			name: "Response Size",
			fields: []string{
				"bytes_sent",
				"body_bytes_sent",
				"O",
				"B",
				"b",
			},
			cases: []subTest{
				{input: "15", wantLine: logLine{web: web{respSize: 15}}},
				{input: "1000000", wantLine: logLine{web: web{respSize: 1000000}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: logLine{web: web{respSize: 0}}},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadRespSize},
				{input: "100.222", wantLine: emptyLogLine, wantErr: errBadRespSize},
				{input: "invalid", wantLine: emptyLogLine, wantErr: errBadRespSize},
			},
		},
		{
			name: "Request Processing Time",
			fields: []string{
				"request_time",
				"D",
			},
			cases: []subTest{
				{input: "100222", wantLine: logLine{web: web{reqProcTime: 100222}}},
				{input: "100.222", wantLine: logLine{web: web{reqProcTime: 100222000}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadReqProcTime},
				{input: "0.333,0.444,0.555", wantLine: emptyLogLine, wantErr: errBadReqProcTime},
				{input: "number", wantLine: emptyLogLine, wantErr: errBadReqProcTime},
			},
		},
		{
			name: "Upstream Response Time",
			fields: []string{
				"upstream_response_time",
			},
			cases: []subTest{
				{input: "100222", wantLine: logLine{web: web{upsRespTime: 100222}}},
				{input: "100.222", wantLine: logLine{web: web{upsRespTime: 100222000}}},
				{input: "0.100 , 0.400 : 0.200 ", wantLine: logLine{web: web{upsRespTime: 700000}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadUpsRespTime},
				{input: "number", wantLine: emptyLogLine, wantErr: errBadUpsRespTime},
			},
		},
		{
			name: "SSL Protocol",
			fields: []string{
				"ssl_protocol",
			},
			cases: []subTest{
				{input: "SSLv3", wantLine: logLine{web: web{sslProto: "SSLv3"}}},
				{input: "SSLv2", wantLine: logLine{web: web{sslProto: "SSLv2"}}},
				{input: "TLSv1", wantLine: logLine{web: web{sslProto: "TLSv1"}}},
				{input: "TLSv1.1", wantLine: logLine{web: web{sslProto: "TLSv1.1"}}},
				{input: "TLSv1.2", wantLine: logLine{web: web{sslProto: "TLSv1.2"}}},
				{input: "TLSv1.3", wantLine: logLine{web: web{sslProto: "TLSv1.3"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadSSLProto},
				{input: "invalid", wantLine: emptyLogLine, wantErr: errBadSSLProto},
			},
		},
		{
			name: "SSL Cipher Suite",
			fields: []string{
				"ssl_cipher",
			},
			cases: []subTest{
				{input: "ECDHE-RSA-AES256-SHA", wantLine: logLine{web: web{sslCipherSuite: "ECDHE-RSA-AES256-SHA"}}},
				{input: "DHE-RSA-AES256-SHA", wantLine: logLine{web: web{sslCipherSuite: "DHE-RSA-AES256-SHA"}}},
				{input: "AES256-SHA", wantLine: logLine{web: web{sslCipherSuite: "AES256-SHA"}}},
				{input: "PSK-RC4-SHA", wantLine: logLine{web: web{sslCipherSuite: "PSK-RC4-SHA"}}},
				{input: "TLS_AES_256_GCM_SHA384", wantLine: logLine{web: web{sslCipherSuite: "TLS_AES_256_GCM_SHA384"}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadSSLCipherSuite},
				{input: "invalid", wantLine: emptyLogLine, wantErr: errBadSSLCipherSuite},
			},
		},
		{
			name: "Custom Fields",
			fields: []string{
				"custom",
			},
			cases: []subTest{
				{input: "POST", wantLine: logLine{custom: custom{values: []customValue{{name: "custom", value: "POST"}}}}},
				{input: "/example.com", wantLine: logLine{custom: custom{values: []customValue{{name: "custom", value: "/example.com"}}}}},
				{input: "HTTP/1.1", wantLine: logLine{custom: custom{values: []customValue{{name: "custom", value: "HTTP/1.1"}}}}},
				{input: "0.333,0.444,0.555", wantLine: logLine{custom: custom{values: []customValue{{name: "custom", value: "0.333,0.444,0.555"}}}}},
				{input: "-1", wantLine: logLine{custom: custom{values: []customValue{{name: "custom", value: "-1"}}}}},
				{input: "invalid", wantLine: logLine{custom: custom{values: []customValue{{name: "custom", value: "invalid"}}}}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
			},
		},
		{
			name: "Custom Fields Not Exist",
			fields: []string{
				"custom_field_not_exist",
			},
			cases: []subTest{
				{input: "POST", wantLine: emptyLogLine},
				{input: "/example.com", wantLine: emptyLogLine},
				{input: "HTTP/1.1", wantLine: emptyLogLine},
				{input: "0.333,0.444,0.555", wantLine: emptyLogLine},
				{input: "-1", wantLine: emptyLogLine},
				{input: "invalid", wantLine: emptyLogLine},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
			},
		},
	}

	for _, tt := range tests {
		for _, field := range tt.fields {
			for i, tc := range tt.cases {
				name := fmt.Sprintf("[%s:%d]field='%s'|line='%s'", tt.name, i+1, field, tc.input)
				t.Run(name, func(t *testing.T) {

					line := newEmptyLogLineWithFields()
					err := line.Assign(field, tc.input)

					if tc.wantErr != nil {
						require.Error(t, err)
						assert.Truef(t, errors.Is(err, tc.wantErr), "expected '%v' error, got '%v'", tc.wantErr, err)
					} else {
						require.NoError(t, err)
					}

					expected := prepareLogLine(field, tc.wantLine)
					assert.Equal(t, expected, *line)
				})
			}
		}
	}
}

func TestLogLine_verify(t *testing.T) {
	type subTest struct {
		line    logLine
		wantErr error
	}
	tests := []struct {
		name  string
		field string
		cases []subTest
	}{
		{
			name:  "Vhost",
			field: "host",
			cases: []subTest{
				{line: logLine{web: web{vhost: "192.168.0.1"}}},
				{line: logLine{web: web{vhost: "debian10.debian"}}},
				{line: logLine{web: web{vhost: "1ce:1ce::babe"}}},
				{line: logLine{web: web{vhost: "localhost"}}},
				{line: logLine{web: web{vhost: "invalid_vhost"}}, wantErr: errBadVhost},
				{line: logLine{web: web{vhost: "http://192.168.0.1/"}}, wantErr: errBadVhost},
			},
		},
		{
			name:  "Server Port",
			field: "server_port",
			cases: []subTest{
				{line: logLine{web: web{port: "80"}}},
				{line: logLine{web: web{port: "8081"}}},
				{line: logLine{web: web{port: "79"}}, wantErr: errBadPort},
				{line: logLine{web: web{port: "50000"}}, wantErr: errBadPort},
				{line: logLine{web: web{port: "0.0.0.0"}}, wantErr: errBadPort},
			},
		},
		{
			name:  "Scheme",
			field: "scheme",
			cases: []subTest{
				{line: logLine{web: web{reqScheme: "http"}}},
				{line: logLine{web: web{reqScheme: "https"}}},
				{line: logLine{web: web{reqScheme: "not_https"}}, wantErr: errBadReqScheme},
				{line: logLine{web: web{reqScheme: "HTTP"}}, wantErr: errBadReqScheme},
				{line: logLine{web: web{reqScheme: "HTTPS"}}, wantErr: errBadReqScheme},
				{line: logLine{web: web{reqScheme: "10"}}, wantErr: errBadReqScheme},
			},
		},
		{
			name:  "Client",
			field: "remote_addr",
			cases: []subTest{
				{line: logLine{web: web{reqClient: "1.1.1.1"}}},
				{line: logLine{web: web{reqClient: "::1"}}},
				{line: logLine{web: web{reqClient: "1ce:1ce::babe"}}},
				{line: logLine{web: web{reqClient: "localhost"}}},
				{line: logLine{web: web{reqClient: "debian10.debian"}}, wantErr: errBadReqClient},
				{line: logLine{web: web{reqClient: "invalid"}}, wantErr: errBadReqClient},
			},
		},
		{
			name:  "Request HTTP Method",
			field: "request_method",
			cases: []subTest{
				{line: logLine{web: web{reqMethod: "GET"}}},
				{line: logLine{web: web{reqMethod: "POST"}}},
				{line: logLine{web: web{reqMethod: "TRACE"}}},
				{line: logLine{web: web{reqMethod: "OPTIONS"}}},
				{line: logLine{web: web{reqMethod: "CONNECT"}}},
				{line: logLine{web: web{reqMethod: "DELETE"}}},
				{line: logLine{web: web{reqMethod: "PUT"}}},
				{line: logLine{web: web{reqMethod: "PATCH"}}},
				{line: logLine{web: web{reqMethod: "HEAD"}}},
				{line: logLine{web: web{reqMethod: "MKCOL"}}},
				{line: logLine{web: web{reqMethod: "PROPFIND"}}},
				{line: logLine{web: web{reqMethod: "MOVE"}}},
				{line: logLine{web: web{reqMethod: "SEARCH"}}},
				{line: logLine{web: web{reqMethod: "Get"}}, wantErr: errBadReqMethod},
				{line: logLine{web: web{reqMethod: "get"}}, wantErr: errBadReqMethod},
			},
		},
		{
			name:  "Request URL",
			field: "request_uri",
			cases: []subTest{
				{line: logLine{web: web{reqURL: "/"}}},
				{line: logLine{web: web{reqURL: "/status?full&json"}}},
				{line: logLine{web: web{reqURL: "/icons/openlogo-75.png"}}},
				{line: logLine{web: web{reqURL: "status?full&json"}}},
				{line: logLine{web: web{reqURL: "\"req_url=/ \""}}},
				{line: logLine{web: web{reqURL: "http://192.168.0.1/"}}},
				{line: logLine{web: web{reqURL: ""}}},
			},
		},
		{
			name:  "Request HTTP Protocol",
			field: "server_protocol",
			cases: []subTest{
				{line: logLine{web: web{reqProto: "1"}}},
				{line: logLine{web: web{reqProto: "1.0"}}},
				{line: logLine{web: web{reqProto: "1.1"}}},
				{line: logLine{web: web{reqProto: "2.0"}}},
				{line: logLine{web: web{reqProto: "2"}}},
				{line: logLine{web: web{reqProto: "0.9"}}, wantErr: errBadReqProto},
				{line: logLine{web: web{reqProto: "1.1.1"}}, wantErr: errBadReqProto},
				{line: logLine{web: web{reqProto: "2.2"}}, wantErr: errBadReqProto},
				{line: logLine{web: web{reqProto: "localhost"}}, wantErr: errBadReqProto},
			},
		},
		{
			name:  "Response Status Code",
			field: "status",
			cases: []subTest{
				{line: logLine{web: web{respCode: 100}}},
				{line: logLine{web: web{respCode: 200}}},
				{line: logLine{web: web{respCode: 300}}},
				{line: logLine{web: web{respCode: 400}}},
				{line: logLine{web: web{respCode: 500}}},
				{line: logLine{web: web{respCode: 600}}},
				{line: logLine{web: web{respCode: -1}}, wantErr: errBadRespCode},
				{line: logLine{web: web{respCode: 99}}, wantErr: errBadRespCode},
				{line: logLine{web: web{respCode: 601}}, wantErr: errBadRespCode},
			},
		},
		{
			name:  "Request size",
			field: "request_length",
			cases: []subTest{
				{line: logLine{web: web{reqSize: 0}}},
				{line: logLine{web: web{reqSize: 100}}},
				{line: logLine{web: web{reqSize: 1000000}}},
				{line: logLine{web: web{reqSize: -1}}, wantErr: errBadReqSize},
			},
		},
		{
			name:  "Response size",
			field: "bytes_sent",
			cases: []subTest{
				{line: logLine{web: web{respSize: 0}}},
				{line: logLine{web: web{respSize: 100}}},
				{line: logLine{web: web{respSize: 1000000}}},
				{line: logLine{web: web{respSize: -1}}, wantErr: errBadRespSize},
			},
		},
		{
			name:  "Request Processing Time",
			field: "request_time",
			cases: []subTest{
				{line: logLine{web: web{reqProcTime: 0}}},
				{line: logLine{web: web{reqProcTime: 100}}},
				{line: logLine{web: web{reqProcTime: 1000.123}}},
				{line: logLine{web: web{reqProcTime: -1}}, wantErr: errBadReqProcTime},
			},
		},
		{
			name:  "Upstream Response Time",
			field: "upstream_response_time",
			cases: []subTest{
				{line: logLine{web: web{upsRespTime: 0}}},
				{line: logLine{web: web{upsRespTime: 100}}},
				{line: logLine{web: web{upsRespTime: 1000.123}}},
				{line: logLine{web: web{upsRespTime: -1}}, wantErr: errBadUpsRespTime},
			},
		},
		{
			name:  "SSL Protocol",
			field: "ssl_protocol",
			cases: []subTest{
				{line: logLine{web: web{sslProto: "SSLv3"}}},
				{line: logLine{web: web{sslProto: "SSLv2"}}},
				{line: logLine{web: web{sslProto: "TLSv1"}}},
				{line: logLine{web: web{sslProto: "TLSv1.1"}}},
				{line: logLine{web: web{sslProto: "TLSv1.2"}}},
				{line: logLine{web: web{sslProto: "TLSv1.3"}}},
				{line: logLine{web: web{sslProto: "invalid"}}, wantErr: errBadSSLProto},
			},
		},
		{
			name:  "SSL Cipher Suite",
			field: "ssl_cipher",
			cases: []subTest{
				{line: logLine{web: web{sslCipherSuite: "ECDHE-RSA-AES256-SHA"}}},
				{line: logLine{web: web{sslCipherSuite: "DHE-RSA-AES256-SHA"}}},
				{line: logLine{web: web{sslCipherSuite: "AES256-SHA"}}},
				{line: logLine{web: web{sslCipherSuite: "TLS_AES_256_GCM_SHA384"}}},
				{line: logLine{web: web{sslCipherSuite: "invalid"}}, wantErr: errBadSSLCipherSuite},
			},
		},
		{
			name:  "Custom Fields",
			field: "custom",
			cases: []subTest{
				{line: logLine{custom: custom{values: []customValue{{name: "custom", value: "POST"}}}}},
				{line: logLine{custom: custom{values: []customValue{{name: "custom", value: "/example.com"}}}}},
				{line: logLine{custom: custom{values: []customValue{{name: "custom", value: "0.333,0.444,0.555"}}}}},
			},
		},
		{
			name: "Empty Line",
			cases: []subTest{
				{line: emptyLogLine, wantErr: errEmptyLine},
			},
		},
	}

	for _, tt := range tests {
		for i, tc := range tt.cases {
			name := fmt.Sprintf("[%s:%d]field='%s'", tt.name, i+1, tt.field)

			t.Run(name, func(t *testing.T) {
				line := prepareLogLine(tt.field, tc.line)

				err := line.verify()

				if tc.wantErr != nil {
					require.Error(t, err)
					assert.Truef(t, errors.Is(err, tc.wantErr), "expected '%v' error, got '%v'", tc.wantErr, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
}

func prepareLogLine(field string, template logLine) logLine {
	if template.empty() {
		return *newEmptyLogLineWithFields()
	}

	line := newEmptyLogLineWithFields()
	line.reset()

	switch field {
	case "host", "http_host", "v":
		line.vhost = template.vhost
	case "server_port", "p":
		line.port = template.port
	case "host:$server_port", "v:%p":
		line.vhost = template.vhost
		line.port = template.port
	case "scheme":
		line.reqScheme = template.reqScheme
	case "remote_addr", "a", "h":
		line.reqClient = template.reqClient
	case "request", "r":
		line.reqMethod = template.reqMethod
		line.reqURL = template.reqURL
		line.reqProto = template.reqProto
	case "request_method", "m":
		line.reqMethod = template.reqMethod
	case "request_uri", "U":
		line.reqURL = template.reqURL
	case "server_protocol", "H":
		line.reqProto = template.reqProto
	case "status", "s", ">s":
		line.respCode = template.respCode
	case "request_length", "I":
		line.reqSize = template.reqSize
	case "bytes_sent", "body_bytes_sent", "b", "O", "B":
		line.respSize = template.respSize
	case "request_time", "D":
		line.reqProcTime = template.reqProcTime
	case "upstream_response_time":
		line.upsRespTime = template.upsRespTime
	case "ssl_protocol":
		line.sslProto = template.sslProto
	case "ssl_cipher":
		line.sslCipherSuite = template.sslCipherSuite
	default:
		line.custom.values = template.custom.values
	}
	return *line
}

func newEmptyLogLineWithFields() *logLine {
	l := newEmptyLogLine()
	l.custom.fields = map[string]struct{}{"custom": {}}
	return l
}
