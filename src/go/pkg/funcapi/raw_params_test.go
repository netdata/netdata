// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRawParams(t *testing.T) {
	tests := map[string]struct {
		req     RawMethodRequest
		want    RawParams
		wantErr string
	}{
		"no input": {
			want: rawParams(map[string]string{}),
		},
		"arguments": {
			req: RawMethodRequest{
				Args: []string{"info", "service:/log", "after=-60", "query:a:b", "before=1=2", "state:", ":x"},
			},
			want: rawParams(map[string]string{
				"service": `"/log"`,
				"after":   `"-60"`,
				"query":   `"a:b"`,
				"before":  `"1=2"`,
				"state":   `""`,
				"":        `"x"`,
			}),
		},
		"last duplicate argument wins": {
			req: RawMethodRequest{
				Args: []string{"state:warning", "state=critical"},
			},
			want: rawParams(map[string]string{"state": `"critical"`}),
		},
		"arguments, then selections, then payload": {
			req: RawMethodRequest{
				Args:    []string{"service:/argument"},
				Payload: []byte(`{"service":"/payload","severity":"OK","after":-60,"selections":{"service":["/selection"],"severity":["Warning"]}}`),
			},
			want: rawParams(map[string]string{
				"service":    `"/argument"`,
				"severity":   `["Warning"]`,
				"after":      `-60`,
				"selections": `{"service":["/selection"],"severity":["Warning"]}`,
			}),
		},
		"null selections": {
			req: RawMethodRequest{
				Payload: []byte(`{"selections":null,"state":"all"}`),
			},
			want: rawParams(map[string]string{"selections": `null`, "state": `"all"`}),
		},
		"empty payload object": {
			req: RawMethodRequest{
				Payload: []byte(`{}`),
			},
			want: rawParams(map[string]string{}),
		},
		"invalid JSON": {
			req: RawMethodRequest{
				Payload: []byte(`[`),
			},
			wantErr: "expected a JSON object",
		},
		"array payload": {
			req: RawMethodRequest{
				Payload: []byte(`[]`),
			},
			wantErr: "expected a JSON object",
		},
		"null payload": {
			req: RawMethodRequest{
				Payload: []byte(`null`),
			},
			wantErr: "expected a JSON object",
		},
		"blank payload": {
			req: RawMethodRequest{
				Payload: []byte(" "),
			},
			wantErr: "expected a JSON object",
		},
		"selections not an object": {
			req: RawMethodRequest{
				Payload: []byte(`{"selections":["a"]}`),
			},
			wantErr: "invalid selections",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ParseRawParams(tc.req)
			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
				assert.Equal(t, RawParams{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRawParams_String(t *testing.T) {
	tests := map[string]struct {
		raw     *string // nil: the key is absent
		want    string
		wantErr string
	}{
		"absent":                {want: ""},
		"string":                {raw: new(`"/log"`), want: "/log"},
		"empty string":          {raw: new(`""`), want: ""},
		"single selection":      {raw: new(`["Warning"]`), want: "Warning"},
		"empty selection value": {raw: new(`[""]`), want: ""},
		"no selection":          {raw: new(`[]`), wantErr: "key requires one string value"},
		"several selections":    {raw: new(`["a","b"]`), wantErr: "key requires one string value"},
		"null":                  {raw: new(`null`), wantErr: "key requires one string value"},
		"number":                {raw: new(`1`), wantErr: "key requires one string value"},
		"object":                {raw: new(`{"a":"b"}`), wantErr: "key requires one string value"},
		"number selection":      {raw: new(`[1]`), wantErr: "key requires one string value"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := rawParams(map[string]string{})
			if tc.raw != nil {
				p = rawParams(map[string]string{"key": *tc.raw})
			}
			got, err := p.String("key")
			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRawParams_TimeRange(t *testing.T) {
	now := time.Unix(1700000000, 0)
	type window struct {
		after, before *int64
	}
	tests := map[string]struct {
		req     RawMethodRequest
		want    window
		wantErr string
	}{
		"absent": {},
		"absolute": {
			req:  payloadRequest(`{"after":1699990000,"before":1699999000}`),
			want: window{after: new(int64(1699990000)), before: new(int64(1699999000))},
		},
		"equal bounds": {
			req:  payloadRequest(`{"after":10,"before":10}`),
			want: window{after: new(int64(10)), before: new(int64(10))},
		},
		"arguments relative to now": {
			req: RawMethodRequest{
				Args: []string{"after:-3600", "before:0"},
			},
			want: window{after: new(now.Unix() - 3600), before: new(now.Unix())},
		},
		"after relative to relative before": {
			req:  payloadRequest(`{"after":-60,"before":-600}`),
			want: window{after: new(now.Unix() - 660), before: new(now.Unix() - 600)},
		},
		"after relative to absolute before": {
			req:  payloadRequest(`{"after":-60,"before":1600000000}`),
			want: window{after: new(int64(1599999940)), before: new(int64(1600000000))},
		},
		"after relative to now without before": {
			req:  payloadRequest(`{"after":-60}`),
			want: window{after: new(now.Unix() - 60)},
		},
		"after zero is the epoch": {
			req:  payloadRequest(`{"after":0,"before":1}`),
			want: window{after: new(int64(0)), before: new(int64(1))},
		},
		"quoted seconds": {
			req:  payloadRequest(`{"after":"-60","before":"1700000000"}`),
			want: window{after: new(now.Unix() - 60), before: new(now.Unix())},
		},
		"range bounds": {
			req:  payloadRequest(`{"after":-62135596800,"before":253402300799}`),
			want: window{after: new(int64(253402300799 - 62135596800)), before: new(int64(253402300799))},
		},
		"lowest relative values": {
			req:  payloadRequest(`{"after":-62135596800,"before":-62135596800}`),
			want: window{after: new(int64(-62135596800*2 + 1700000000)), before: new(int64(-62135596800 + 1700000000))},
		},
		"reversed": {
			req:     payloadRequest(`{"after":10,"before":9}`),
			wantErr: "after must not exceed before",
		},
		"not a number": {
			req:     payloadRequest(`{"after":"bad"}`),
			wantErr: "after must be integer Unix seconds or relative seconds",
		},
		"fractional": {
			req:     payloadRequest(`{"before":1.5}`),
			wantErr: "before must be integer Unix seconds or relative seconds",
		},
		"quoted fraction": {
			req:     payloadRequest(`{"after":"1.5"}`),
			wantErr: "after must be integer Unix seconds or relative seconds",
		},
		"null": {
			req:     payloadRequest(`{"after":null}`),
			wantErr: "after must be integer Unix seconds or relative seconds",
		},
		"below range": {
			req:     payloadRequest(`{"after":-62135596801}`),
			wantErr: "after is outside the supported timestamp range",
		},
		"above range": {
			req:     payloadRequest(`{"before":253402300800}`),
			wantErr: "before is outside the supported timestamp range",
		},
		"int64 overflow": {
			req:     payloadRequest(`{"before":"9223372036854775808"}`),
			wantErr: "before must be integer Unix seconds or relative seconds",
		},
		"after error first": {
			req:     payloadRequest(`{"after":"bad","before":"bad"}`),
			wantErr: "after must be integer Unix seconds or relative seconds",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := ParseRawParams(tc.req)
			require.NoError(t, err)
			var got window
			got.after, got.before, err = p.TimeRange(now)
			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRawParams_TimeRangeInvalidQuotedValue(t *testing.T) {
	// A parsed payload always holds valid JSON; this covers a malformed stored value.
	after, before, err := rawParams(map[string]string{"after": `"\q"`}).TimeRange(time.Unix(0, 0))
	assert.EqualError(t, err, "invalid after time")
	assert.Nil(t, after)
	assert.Nil(t, before)
}

func TestRawParams_QueryMatcher(t *testing.T) {
	texts := []string{"Fan failed", "Fan normal", "PSU failed"}
	tests := map[string]struct {
		req RawMethodRequest
		// wantMatched lists the texts the matcher accepts; nil means no matcher.
		wantMatched []string
		wantErr     string
	}{
		"absent": {},
		"blank": {
			req: payloadRequest(`{"query":"  "}`),
		},
		"pattern": {
			req:         payloadRequest(`{"query":"*failed*"}`),
			wantMatched: []string{"Fan failed", "PSU failed"},
		},
		"case-sensitive": {
			req:         payloadRequest(`{"query":"*FAILED*"}`),
			wantMatched: []string{},
		},
		"ordered exclusion": {
			req:         payloadRequest(`{"query":"!*failed* *Fan*"}`),
			wantMatched: []string{"Fan normal"},
		},
		"argument": {
			req: RawMethodRequest{
				Args: []string{"query:PSU*"},
			},
			wantMatched: []string{"PSU failed"},
		},
		"single selection": {
			req:         payloadRequest(`{"selections":{"query":["Fan*"]}}`),
			wantMatched: []string{"Fan failed", "Fan normal"},
		},
		"invalid pattern": {
			req:     payloadRequest(`{"query":"["}`),
			wantErr: "invalid simple-pattern query",
		},
		"several patterns": {
			req:     payloadRequest(`{"query":["a","b"]}`),
			wantErr: "query requires one string value",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := ParseRawParams(tc.req)
			require.NoError(t, err)
			m, err := p.QueryMatcher()
			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			var matched []string
			if m != nil {
				matched = []string{}
				for _, text := range texts {
					if m.MatchString(text) {
						matched = append(matched, text)
					}
				}
			}
			assert.Equal(t, tc.wantMatched, matched)
		})
	}
}

func rawParams(values map[string]string) RawParams {
	p := RawParams{values: make(map[string]json.RawMessage, len(values))}
	for k, v := range values {
		p.values[k] = json.RawMessage(v)
	}
	return p
}

func payloadRequest(payload string) RawMethodRequest {
	return RawMethodRequest{
		Payload: []byte(payload),
	}
}
