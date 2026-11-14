// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

var defaultRedactor = redact.NewRedactor()

var viewURLScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// Text is normalized before it becomes an aggregation key or retained evidence.
// Opaque protocol IDs use their own validation and are not generalized as paths.
func (o Options) apply(value string) string {
	r := o.Redactor
	if r == nil {
		r = defaultRedactor
	}
	return r.Apply(value)
}

func (o Options) text(value string, limit int) string {
	return clean(o.apply(value), limit)
}

func (o Options) cleanURL(value string) string {
	if value == "" {
		return ""
	}
	value = o.apply(redactURL(value, o.PathRules))
	// Keep the full URL until callers separate host and path or bound an attribute.
	return clean(value, len(value))
}

func (o Options) url(value string, limit int) string {
	return truncate(o.cleanURL(value), limit)
}

func (o Options) view(value string) string {
	value = clean(value, len(value))
	if strings.Contains(value, "/") || viewURLScheme.MatchString(strings.TrimSpace(value)) {
		value = o.url(value, maxPathLen)
		if value != "" {
			value = urlPath(value)
		}
	} else {
		for _, rule := range o.PathRules {
			value = rule.Re.ReplaceAllString(value, rule.Replace)
		}
	}
	return o.text(value, maxNameLen)
}

func (o Options) selector(value string) string {
	// Apply credential patterns before the selector and size transformations.
	return clean(redactSelector(o.apply(value)), maxSelector)
}

type stackFrame struct {
	Filename string `json:"filename"`
	Function string `json:"function"`
	Lineno   int    `json:"lineno"`
	Colno    int    `json:"colno"`
}

func (o Options) appendFrame(stack *strings.Builder, frame stackFrame) string {
	function := o.text(frame.Function, maxStackLen)
	filename := o.text(redactSourceURL(frame.Filename, o.PathRules), maxStackLen)
	fmt.Fprintf(stack, "%s (%s:%d:%d)\n", function, filename, frame.Lineno, frame.Colno)
	return function + "@" + filename
}

// attrSemantic is how an event attribute value is read.
type attrSemantic uint8

const (
	attrByKey    attrSemantic = iota // by its key, like any attribute (cleanAttr)
	attrID                           // opaque observation identity
	attrRevision                     // observation sequence
	attrNumber                       // finite non-negative number
	attrURL                          // URL without query or fragment
)

// attrRule says how to read one event attribute. A native attribute serves
// measurements: it is kept with optional event logs off and its key is never
// rewritten as free text.
type attrRule struct {
	semantic attrSemantic
	native   bool
}

func nativeAttr(semantic attrSemantic) attrRule { return attrRule{semantic: semantic, native: true} }

// eventAttrRule declares, per event kind, the protocol attributes measurements
// read. Values are interpreted by their original key, before custom-key
// redaction can change its meaning. Every other attribute is diagnostic text.
func eventAttrRule(kind beacon.EventKind, name, key string) attrRule {
	if key == "observation_id" || key == "observation_sequence" {
		switch kind {
		case beacon.EventDocument, beacon.EventView, beacon.EventNavigation, beacon.EventResource, beacon.EventSession:
			if key == "observation_id" {
				return nativeAttr(attrID)
			}
			return nativeAttr(attrRevision)
		}
		return attrRule{}
	}
	switch kind {
	case beacon.EventNavigation:
		switch key {
		case "pageLoadTime", "domContentLoadHandlerTime":
			return nativeAttr(attrNumber)
		case "name":
			return attrRule{semantic: attrURL}
		}
	case beacon.EventResource:
		switch key {
		case "name":
			return nativeAttr(attrURL)
		case "duration", "transferSize":
			return nativeAttr(attrNumber)
		case "httpHost", "initiatorType":
			return nativeAttr(attrByKey)
		}
	case beacon.EventRequest:
		switch key {
		case "http.response.status_code", "duration_ns":
			return nativeAttr(attrNumber)
		case "url.full", "http.request.method":
			return nativeAttr(attrByKey)
		}
	case beacon.EventView:
		switch key {
		case "fromView", "toView":
			return nativeAttr(attrByKey)
		}
	case beacon.EventCustom:
		// Frustration signals are custom events; their target is the element.
		if key == "target" && beacon.IsFrustration(name) {
			return nativeAttr(attrByKey)
		}
	}
	return attrRule{}
}

// eventAttr reads value under rule. Malformed identities and numbers are
// dropped: numeric protocol values are measurements, not free text.
func (o Options) eventAttr(rule attrRule, key, value string) (string, bool) {
	switch rule.semantic {
	case attrID:
		id := observationID(value)
		return id, id != ""
	case attrRevision:
		return value, observationRevision(value) != 0
	case attrNumber:
		value = clean(value, len(value))
		if _, ok := parseNonNegative(value); !ok {
			return "", false
		}
		return truncate(value, maxAttrLen), true
	case attrURL:
		return o.url(value, maxAttrLen), true
	default:
		return o.cleanAttr(key, value), true
	}
}

// urlAttrKeys hold URLs; they lose query and fragment like page URLs.
var urlAttrKeys = map[string]bool{"url.full": true, "http.url": true, "url": true, "name": true}

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

// parseNonNegative reads a finite, non-negative protocol number; anything else
// is absent.
func parseNonNegative(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || !nonNegativeFinite(v) {
		return 0, false
	}
	return v, true
}

func nonNegativeFinite(v float64) bool { return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

// IDs are opaque and must survive unchanged. Reject, rather than truncate or
// redact, malformed values so unrelated observations cannot collapse together.
func observationID(value string) string {
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.' || c == ':') {
			return ""
		}
	}
	return value
}

func observationRevision(value string) uint64 {
	// The producer serializes positive Number-safe integers in canonical decimal.
	if len(value) == 0 || len(value) > 16 || value[0] < '1' || value[0] > '9' {
		return 0
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || n == 0 || n > 9007199254740991 {
		return 0
	}
	return n
}

func observationContextID(ctx map[string]any) string {
	value, _ := ctx["id"].(string)
	return observationID(value)
}

func observationContextRevision(ctx map[string]any) uint64 {
	value, _ := ctx["observation_sequence"].(string)
	return observationRevision(value)
}
