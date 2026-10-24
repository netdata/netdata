// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// OTLP/JSON shapes as Faro's tracing add-on sends them: IDs are hex,
// nanosecond times and integers are JSON strings or numbers.
type otlpKV struct {
	Key   string `json:"key"`
	Value struct {
		StringValue *string         `json:"stringValue"`
		IntValue    json.RawMessage `json:"intValue"`
		BoolValue   *bool           `json:"boolValue"`
		DoubleValue *float64        `json:"doubleValue"`
	} `json:"value"`
}

type otlpSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId"`
	Name              string          `json:"name"`
	Kind              int32           `json:"kind"`
	StartTimeUnixNano json.RawMessage `json:"startTimeUnixNano"`
	EndTimeUnixNano   json.RawMessage `json:"endTimeUnixNano"`
	Attributes        []otlpKV        `json:"attributes"`
	Status            struct {
		Code    int32  `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
}

type otlpResourceSpans struct {
	Resource struct {
		Attributes []otlpKV `json:"attributes"`
	} `json:"resource"`
	ScopeSpans []struct {
		Scope struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"scope"`
		Spans []otlpSpan `json:"spans"`
	} `json:"scopeSpans"`
}

const (
	maxSpans     = 100
	maxSpanAttrs = 32
)

// urlAttrKeys hold URLs; they lose query and fragment like page URLs.
var urlAttrKeys = map[string]bool{"url.full": true, "http.url": true, "url": true, "name": true}

// parseSpans keeps well-formed spans, bounded and with URLs stripped, and
// the first resource's service.name. Browser resource attributes (user
// agent, brands, language) are dropped; the exporter sets its own.
func parseSpans(rs []otlpResourceSpans, opt Options) ([]beacon.Span, string) {
	var out []beacon.Span
	service := ""
	for _, r := range rs {
		for _, kv := range r.Resource.Attributes {
			if kv.Key == "service.name" && kv.Value.StringValue != nil && service == "" {
				service = opt.text(*kv.Value.StringValue, maxNameLen)
			}
		}
		for _, ss := range r.ScopeSpans {
			for _, sp := range ss.Spans {
				if len(out) >= maxSpans {
					return out, service
				}
				traceID, spanID := hexID(sp.TraceID, 32), hexID(sp.SpanID, 16)
				start, end := nanos(sp.StartTimeUnixNano), nanos(sp.EndTimeUnixNano)
				if traceID == "" || spanID == "" || start == 0 || end < start {
					continue
				}
				out = append(out, beacon.Span{
					TraceID:       traceID,
					SpanID:        spanID,
					ParentSpanID:  hexID(sp.ParentSpanID, 16),
					Name:          opt.text(sp.Name, maxNameLen),
					Kind:          sp.Kind,
					StartNS:       start,
					EndNS:         end,
					Attrs:         spanAttrs(sp.Attributes, opt),
					StatusCode:    sp.Status.Code,
					StatusMessage: opt.text(sp.Status.Message, maxAttrLen),
					Scope: opt.text(
						ss.Scope.Name,
						maxNameLen,
					),
					ScopeVersion: opt.text(ss.Scope.Version, maxNameLen),
				})
			}
		}
	}
	return out, service
}

func spanAttrs(kvs []otlpKV, opt Options) []beacon.SpanAttr {
	out := make([]beacon.SpanAttr, 0, min(len(kvs), maxSpanAttrs))
	for _, kv := range kvs {
		if len(out) >= maxSpanAttrs {
			break
		}
		semanticKey := beacon.Clean(kv.Key, len(kv.Key))
		key := opt.text(semanticKey, maxNameLen)
		if key == "" {
			continue
		}
		v := kv.Value
		switch {
		case v.StringValue != nil:
			out = append(out, beacon.SpanAttr{
				Key: key,
				Str: opt.cleanAttr(semanticKey, *v.StringValue),
			})
		case len(v.IntValue) > 0:
			if n, err := strconv.ParseInt(strings.Trim(string(v.IntValue), `"`), 10, 64); err == nil {
				out = append(out, beacon.SpanAttr{
					Key:   key,
					Int:   n,
					IsInt: true,
				})
			}
		case v.BoolValue != nil:
			out = append(out, beacon.SpanAttr{
				Key: key,
				Str: strconv.FormatBool(*v.BoolValue),
			})
		case v.DoubleValue != nil:
			out = append(out, beacon.SpanAttr{
				Key: key,
				Str: strconv.FormatFloat(*v.DoubleValue, 'f', -1, 64),
			})
		}
	}
	return out
}

// cleanAttr applies field semantics before text bounds and shared ingestion.
func (o Options) cleanAttr(key, value string) string {
	switch {
	case urlAttrKeys[key] && (key != "name" || strings.Contains(value, "://") || strings.HasPrefix(value, "/")):
		return o.url(value, maxAttrLen)
	case key == "target":
		return o.selector(value)
	case key == "fromView" || key == "toView":
		return o.view(value)
	default:
		return o.text(value, maxAttrLen)
	}
}

// hexID returns id lowercased when it is n hex digits and not all zero.
func hexID(id string, n int) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if len(id) != n || strings.Trim(id, "0") == "" {
		return ""
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ""
		}
	}
	return id
}

func nanos(raw json.RawMessage) uint64 {
	n, err := strconv.ParseUint(strings.Trim(string(raw), `"`), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
