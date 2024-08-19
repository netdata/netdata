// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const emptyStr = ""

func TestLogLine_Assign(t *testing.T) {
	type subTest struct {
		input    string
		wantLine logLine
		wantErr  error
	}
	type test struct {
		name  string
		field string
		cases []subTest
	}
	tests := []test{
		{
			name:  "Response Time",
			field: fieldRespTime,
			cases: []subTest{
				{input: "0", wantLine: logLine{respTime: 0}},
				{input: "1000", wantLine: logLine{respTime: 1000}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadRespTime},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadRespTime},
				{input: "0.000", wantLine: emptyLogLine, wantErr: errBadRespTime},
			},
		},
		{
			name:  "Client Address",
			field: fieldClientAddr,
			cases: []subTest{
				{input: "127.0.0.1", wantLine: logLine{clientAddr: "127.0.0.1"}},
				{input: "::1", wantLine: logLine{clientAddr: "::1"}},
				{input: "kadr20.m1.netdata.lan", wantLine: logLine{clientAddr: "kadr20.m1.netdata.lan"}},
				{input: "±!@#$%^&*()", wantLine: logLine{clientAddr: "±!@#$%^&*()"}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadClientAddr},
			},
		},
		{
			name:  "Cache Code",
			field: fieldCacheCode,
			cases: []subTest{
				{input: "TCP_MISS", wantLine: logLine{cacheCode: "TCP_MISS"}},
				{input: "TCP_DENIED", wantLine: logLine{cacheCode: "TCP_DENIED"}},
				{input: "TCP_CLIENT_REFRESH_MISS", wantLine: logLine{cacheCode: "TCP_CLIENT_REFRESH_MISS"}},
				{input: "UDP_MISS_NOFETCH", wantLine: logLine{cacheCode: "UDP_MISS_NOFETCH"}},
				{input: "UDP_INVALID", wantLine: logLine{cacheCode: "UDP_INVALID"}},
				{input: "NONE", wantLine: logLine{cacheCode: "NONE"}},
				{input: "NONE_NONE", wantLine: logLine{cacheCode: "NONE_NONE"}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadCacheCode},
				{input: "TCP", wantLine: emptyLogLine, wantErr: errBadCacheCode},
				{input: "UDP_", wantLine: emptyLogLine, wantErr: errBadCacheCode},
				{input: "NONE_MISS", wantLine: emptyLogLine, wantErr: errBadCacheCode},
			},
		},
		{
			name:  "HTTP Code",
			field: fieldHTTPCode,
			cases: []subTest{
				{input: "000", wantLine: logLine{httpCode: 0}},
				{input: "100", wantLine: logLine{httpCode: 100}},
				{input: "200", wantLine: logLine{httpCode: 200}},
				{input: "300", wantLine: logLine{httpCode: 300}},
				{input: "400", wantLine: logLine{httpCode: 400}},
				{input: "500", wantLine: logLine{httpCode: 500}},
				{input: "603", wantLine: logLine{httpCode: 603}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadHTTPCode},
				{input: "1", wantLine: emptyLogLine, wantErr: errBadHTTPCode},
				{input: "604", wantLine: emptyLogLine, wantErr: errBadHTTPCode},
				{input: "1000", wantLine: emptyLogLine, wantErr: errBadHTTPCode},
				{input: "TCP_MISS", wantLine: emptyLogLine, wantErr: errBadHTTPCode},
			},
		},
		{
			name:  "Response Size",
			field: fieldRespSize,
			cases: []subTest{
				{input: "0", wantLine: logLine{respSize: 0}},
				{input: "1000", wantLine: logLine{respSize: 1000}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadRespSize},
				{input: "-1", wantLine: emptyLogLine, wantErr: errBadRespSize},
				{input: "0.000", wantLine: emptyLogLine, wantErr: errBadRespSize},
			},
		},
		{
			name:  "Request Method",
			field: fieldReqMethod,
			cases: []subTest{
				{input: "GET", wantLine: logLine{reqMethod: "GET"}},
				{input: "HEAD", wantLine: logLine{reqMethod: "HEAD"}},
				{input: "POST", wantLine: logLine{reqMethod: "POST"}},
				{input: "PUT", wantLine: logLine{reqMethod: "PUT"}},
				{input: "PATCH", wantLine: logLine{reqMethod: "PATCH"}},
				{input: "DELETE", wantLine: logLine{reqMethod: "DELETE"}},
				{input: "CONNECT", wantLine: logLine{reqMethod: "CONNECT"}},
				{input: "OPTIONS", wantLine: logLine{reqMethod: "OPTIONS"}},
				{input: "TRACE", wantLine: logLine{reqMethod: "TRACE"}},
				{input: "ICP_QUERY", wantLine: logLine{reqMethod: "ICP_QUERY"}},
				{input: "PURGE", wantLine: logLine{reqMethod: "PURGE"}},
				{input: "PROPFIND", wantLine: logLine{reqMethod: "PROPFIND"}},
				{input: "PROPATCH", wantLine: logLine{reqMethod: "PROPATCH"}},
				{input: "MKCOL", wantLine: logLine{reqMethod: "MKCOL"}},
				{input: "COPY", wantLine: logLine{reqMethod: "COPY"}},
				{input: "MOVE", wantLine: logLine{reqMethod: "MOVE"}},
				{input: "LOCK", wantLine: logLine{reqMethod: "LOCK"}},
				{input: "UNLOCK", wantLine: logLine{reqMethod: "UNLOCK"}},
				{input: "NONE", wantLine: logLine{reqMethod: "NONE"}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadReqMethod},
				{input: "get", wantLine: emptyLogLine, wantErr: errBadReqMethod},
				{input: "0.000", wantLine: emptyLogLine, wantErr: errBadReqMethod},
				{input: "TCP_MISS", wantLine: emptyLogLine, wantErr: errBadReqMethod},
			},
		},
		{
			name:  "Hier Code",
			field: fieldHierCode,
			cases: []subTest{
				{input: "HIER_NONE", wantLine: logLine{hierCode: "HIER_NONE"}},
				{input: "HIER_SIBLING_HIT", wantLine: logLine{hierCode: "HIER_SIBLING_HIT"}},
				{input: "HIER_NO_CACHE_DIGEST_DIRECT", wantLine: logLine{hierCode: "HIER_NO_CACHE_DIGEST_DIRECT"}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadHierCode},
				{input: "0.000", wantLine: emptyLogLine, wantErr: errBadHierCode},
				{input: "TCP_MISS", wantLine: emptyLogLine, wantErr: errBadHierCode},
				{input: "HIER", wantLine: emptyLogLine, wantErr: errBadHierCode},
				{input: "HIER_", wantLine: emptyLogLine, wantErr: errBadHierCode},
				{input: "NONE", wantLine: emptyLogLine, wantErr: errBadHierCode},
				{input: "SIBLING_HIT", wantLine: emptyLogLine, wantErr: errBadHierCode},
				{input: "NO_CACHE_DIGEST_DIRECT", wantLine: emptyLogLine, wantErr: errBadHierCode},
			},
		},
		{
			name:  "Server Address",
			field: fieldServerAddr,
			cases: []subTest{
				{input: "127.0.0.1", wantLine: logLine{serverAddr: "127.0.0.1"}},
				{input: "::1", wantLine: logLine{serverAddr: "::1"}},
				{input: "kadr20.m1.netdata.lan", wantLine: logLine{serverAddr: "kadr20.m1.netdata.lan"}},
				{input: "±!@#$%^&*()", wantLine: logLine{serverAddr: "±!@#$%^&*()"}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
			},
		},
		{
			name:  "Mime Type",
			field: fieldMimeType,
			cases: []subTest{
				{input: "application/zstd", wantLine: logLine{mimeType: "application"}},
				{input: "audio/3gpp2", wantLine: logLine{mimeType: "audio"}},
				{input: "font/otf", wantLine: logLine{mimeType: "font"}},
				{input: "image/tiff", wantLine: logLine{mimeType: "image"}},
				{input: "message/global", wantLine: logLine{mimeType: "message"}},
				{input: "model/example", wantLine: logLine{mimeType: "model"}},
				{input: "multipart/encrypted", wantLine: logLine{mimeType: "multipart"}},
				{input: "text/html", wantLine: logLine{mimeType: "text"}},
				{input: "video/3gpp", wantLine: logLine{mimeType: "video"}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine},
				{input: "example/example", wantLine: emptyLogLine},
				{input: "unknown/example", wantLine: emptyLogLine},
				{input: "audio", wantLine: emptyLogLine, wantErr: errBadMimeType},
				{input: "/", wantLine: emptyLogLine, wantErr: errBadMimeType},
			},
		},
		{
			name:  "Result Code",
			field: fieldResultCode,
			cases: []subTest{
				{input: "TCP_MISS/000", wantLine: logLine{cacheCode: "TCP_MISS", httpCode: 0}},
				{input: "TCP_DENIED/603", wantLine: logLine{cacheCode: "TCP_DENIED", httpCode: 603}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadResultCode},
				{input: "TCP_MISS:000", wantLine: emptyLogLine, wantErr: errBadResultCode},
				{input: "TCP_MISS 000", wantLine: emptyLogLine, wantErr: errBadResultCode},
				{input: "/", wantLine: emptyLogLine, wantErr: errBadResultCode},
				{input: "tcp/000", wantLine: emptyLogLine, wantErr: errBadCacheCode},
				{input: "TCP_MISS/", wantLine: logLine{cacheCode: "TCP_MISS", httpCode: emptyNumber}, wantErr: errBadHTTPCode},
			},
		},
		{
			name:  "Hierarchy",
			field: fieldHierarchy,
			cases: []subTest{
				{input: "HIER_NONE/-", wantLine: logLine{hierCode: "HIER_NONE", serverAddr: emptyString}},
				{input: "HIER_SIBLING_HIT/127.0.0.1", wantLine: logLine{hierCode: "HIER_SIBLING_HIT", serverAddr: "127.0.0.1"}},
				{input: emptyStr, wantLine: emptyLogLine},
				{input: hyphen, wantLine: emptyLogLine, wantErr: errBadHierarchy},
				{input: "HIER_NONE:-", wantLine: emptyLogLine, wantErr: errBadHierarchy},
				{input: "HIER_SIBLING_HIT 127.0.0.1", wantLine: emptyLogLine, wantErr: errBadHierarchy},
				{input: "/", wantLine: emptyLogLine, wantErr: errBadHierarchy},
				{input: "HIER/-", wantLine: emptyLogLine, wantErr: errBadHierCode},
				{input: "HIER_NONE/", wantLine: logLine{hierCode: "HIER_NONE", serverAddr: emptyStr}},
			},
		},
	}

	for _, tt := range tests {
		for i, tc := range tt.cases {
			name := fmt.Sprintf("[%s:%d]field='%s'|input='%s'", tt.name, i+1, tt.field, tc.input)
			t.Run(name, func(t *testing.T) {

				line := newEmptyLogLine()
				err := line.Assign(tt.field, tc.input)

				if tc.wantErr != nil {
					require.Error(t, err)
					assert.Truef(t, errors.Is(err, tc.wantErr), "expected '%v' error, got '%v'", tc.wantErr, err)
				} else {
					require.NoError(t, err)
				}

				expected := prepareAssignLogLine(t, tt.field, tc.wantLine)
				assert.Equal(t, expected, *line)
			})
		}
	}
}

func TestLogLine_verify(t *testing.T) {
	type subTest struct {
		input   string
		wantErr error
	}
	type test = struct {
		name  string
		field string
		cases []subTest
	}
	tests := []test{
		{
			name:  "Response Time",
			field: fieldRespTime,
			cases: []subTest{
				{input: "0"},
				{input: "1000"},
				{input: "-1", wantErr: errBadRespTime},
			},
		},
		{
			name:  "Client Address",
			field: fieldClientAddr,
			cases: []subTest{
				{input: "127.0.0.1"},
				{input: "::1"},
				{input: "kadr20.m1.netdata.lan"},
				{input: emptyStr},
				{input: "±!@#$%^&*()", wantErr: errBadClientAddr},
			},
		},
		{
			name:  "Cache Code",
			field: fieldCacheCode,
			cases: []subTest{
				{input: "TCP_MISS"},
				{input: "TCP_DENIED"},
				{input: "TCP_CLIENT_REFRESH_MISS"},
				{input: "UDP_MISS_NOFETCH"},
				{input: "UDP_INVALID"},
				{input: "NONE"},
				{input: "NONE_NONE"},
				{input: emptyStr},
				{input: "TCP", wantErr: errBadCacheCode},
				{input: "UDP", wantErr: errBadCacheCode},
				{input: "NONE_MISS", wantErr: errBadCacheCode},
			},
		},
		{
			name:  "HTTP Code",
			field: fieldHTTPCode,
			cases: []subTest{
				{input: "000"},
				{input: "100"},
				{input: "200"},
				{input: "300"},
				{input: "400"},
				{input: "500"},
				{input: "603"},
				{input: "1", wantErr: errBadHTTPCode},
				{input: "604", wantErr: errBadHTTPCode},
			},
		},
		{
			name:  "Response Size",
			field: fieldRespSize,
			cases: []subTest{
				{input: "0"},
				{input: "1000"},
				{input: "-1", wantErr: errBadRespSize},
			},
		},
		{
			name:  "Request Method",
			field: fieldReqMethod,
			cases: []subTest{
				{input: "GET"},
				{input: "HEAD"},
				{input: "POST"},
				{input: "PUT"},
				{input: "PATCH"},
				{input: "DELETE"},
				{input: "CONNECT"},
				{input: "OPTIONS"},
				{input: "TRACE"},
				{input: "ICP_QUERY"},
				{input: "PURGE"},
				{input: "PROPFIND"},
				{input: "PROPATCH"},
				{input: "MKCOL"},
				{input: "COPY"},
				{input: "MOVE"},
				{input: "LOCK"},
				{input: "UNLOCK"},
				{input: "NONE"},
				{input: emptyStr},
				{input: "get", wantErr: errBadReqMethod},
				{input: "TCP_MISS", wantErr: errBadReqMethod},
			},
		},
		{
			name:  "Hier Code",
			field: fieldHierCode,
			cases: []subTest{
				{input: "HIER_NONE"},
				{input: "HIER_SIBLING_HIT"},
				{input: "HIER_NO_CACHE_DIGEST_DIRECT"},
				{input: emptyStr},
				{input: "0.000", wantErr: errBadHierCode},
				{input: "TCP_MISS", wantErr: errBadHierCode},
				{input: "HIER", wantErr: errBadHierCode},
				{input: "HIER_", wantErr: errBadHierCode},
				{input: "NONE", wantErr: errBadHierCode},
				{input: "SIBLING_HIT", wantErr: errBadHierCode},
				{input: "NO_CACHE_DIGEST_DIRECT", wantErr: errBadHierCode},
			},
		},
		{
			name:  "Server Address",
			field: fieldServerAddr,
			cases: []subTest{
				{input: "127.0.0.1"},
				{input: "::1"},
				{input: "kadr20.m1.netdata.lan"},
				{input: emptyStr},
				{input: "±!@#$%^&*()", wantErr: errBadServerAddr},
			},
		},
		{
			name:  "Mime Type",
			field: fieldMimeType,
			cases: []subTest{
				{input: "application"},
				{input: "audio"},
				{input: "font"},
				{input: "image"},
				{input: "message"},
				{input: "model"},
				{input: "multipart"},
				{input: "text"},
				{input: "video"},
				{input: emptyStr},
				{input: "example/example", wantErr: errBadMimeType},
				{input: "unknown", wantErr: errBadMimeType},
				{input: "/", wantErr: errBadMimeType},
			},
		},
	}

	for _, tt := range tests {
		for i, tc := range tt.cases {
			name := fmt.Sprintf("[%s:%d]field='%s'|input='%s'", tt.name, i+1, tt.field, tc.input)
			t.Run(name, func(t *testing.T) {
				line := prepareVerifyLogLine(t, tt.field, tc.input)

				err := line.verify()

				if tc.wantErr != nil {
					require.Error(t, err)
					assert.Truef(t, errors.Is(err, tc.wantErr), "expected '%v' error, got '%v'", tc.wantErr, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func prepareAssignLogLine(t *testing.T, field string, template logLine) logLine {
	t.Helper()
	if template.empty() {
		return template
	}

	var line logLine
	line.reset()

	switch field {
	default:
		t.Errorf("prepareAssignLogLine unknown field: '%s'", field)
	case fieldRespTime:
		line.respTime = template.respTime
	case fieldClientAddr:
		line.clientAddr = template.clientAddr
	case fieldCacheCode:
		line.cacheCode = template.cacheCode
	case fieldHTTPCode:
		line.httpCode = template.httpCode
	case fieldRespSize:
		line.respSize = template.respSize
	case fieldReqMethod:
		line.reqMethod = template.reqMethod
	case fieldHierCode:
		line.hierCode = template.hierCode
	case fieldMimeType:
		line.mimeType = template.mimeType
	case fieldServerAddr:
		line.serverAddr = template.serverAddr
	case fieldResultCode:
		line.cacheCode = template.cacheCode
		line.httpCode = template.httpCode
	case fieldHierarchy:
		line.hierCode = template.hierCode
		line.serverAddr = template.serverAddr
	}
	return line
}

func prepareVerifyLogLine(t *testing.T, field string, value string) logLine {
	t.Helper()
	var line logLine
	line.reset()

	switch field {
	default:
		t.Errorf("prepareVerifyLogLine unknown field: '%s'", field)
	case fieldRespTime:
		v, err := strconv.Atoi(value)
		require.NoError(t, err)
		line.respTime = v
	case fieldClientAddr:
		line.clientAddr = value
	case fieldCacheCode:
		line.cacheCode = value
	case fieldHTTPCode:
		v, err := strconv.Atoi(value)
		require.NoError(t, err)
		line.httpCode = v
	case fieldRespSize:
		v, err := strconv.Atoi(value)
		require.NoError(t, err)
		line.respSize = v
	case fieldReqMethod:
		line.reqMethod = value
	case fieldHierCode:
		line.hierCode = value
	case fieldMimeType:
		line.mimeType = value
	case fieldServerAddr:
		line.serverAddr = value
	}
	return line
}
