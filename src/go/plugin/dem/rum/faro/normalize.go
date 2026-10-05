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
	return beacon.Clean(o.apply(value), limit)
}

func (o Options) cleanURL(value string) string {
	if value == "" {
		return ""
	}
	value = o.apply(beacon.RedactURL(value, o.PathRules))
	// Keep the full URL until callers separate host and path or bound an attribute.
	return beacon.Clean(value, len(value))
}

func (o Options) url(value string, limit int) string {
	return beacon.Truncate(o.cleanURL(value), limit)
}

func (o Options) view(value string) string {
	value = beacon.Clean(value, len(value))
	if strings.Contains(value, "/") || viewURLScheme.MatchString(strings.TrimSpace(value)) {
		value = o.url(value, maxPathLen)
		if value != "" {
			value = beacon.Path(value)
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
	r := o.Redactor
	if r == nil {
		r = defaultRedactor
	}
	return beacon.Clean(beacon.RedactSelector(r.Apply(value)), maxSelector)
}

type stackFrame struct {
	Filename string `json:"filename"`
	Function string `json:"function"`
	Lineno   int    `json:"lineno"`
	Colno    int    `json:"colno"`
}

func (o Options) appendFrame(stack *strings.Builder, frame stackFrame) string {
	function := o.text(frame.Function, maxStackLen)
	filename := o.text(beacon.RedactSourceURL(frame.Filename, o.PathRules), maxStackLen)
	fmt.Fprintf(stack, "%s (%s:%d:%d)\n", function, filename, frame.Lineno, frame.Colno)
	return function + "@" + filename
}

// Typed event attributes serve native measurements even with optional logs off.
func keepEventAttr(kind beacon.EventKind, name, key string) bool {
	switch kind {
	case beacon.EventNavigation:
		return key == "pageLoadTime" || key == "domContentLoadHandlerTime"
	case beacon.EventResource:
		return key == "name" || key == "httpHost" || key == "duration" || key == "transferSize" || key == "initiatorType"
	case beacon.EventRequest:
		return key == "url.full" || key == "http.request.method" || key == "http.response.status_code" || key == "duration_ns"
	case beacon.EventView:
		return key == "fromView" || key == "toView"
	default:
		return beacon.IsFrustration(name) && key == "target"
	}
}

// Numeric protocol values are measurements, not free text. Interpret each field
// by its original key, before custom-key redaction can change that meaning.
func (o Options) eventAttr(kind beacon.EventKind, key, value string) (string, bool) {
	if (kind == beacon.EventResource || kind == beacon.EventNavigation) && key == "name" {
		return o.url(value, maxAttrLen), true
	}
	numeric := false
	switch kind {
	case beacon.EventNavigation:
		numeric = key == "pageLoadTime" || key == "domContentLoadHandlerTime"
	case beacon.EventResource:
		numeric = key == "duration" || key == "transferSize"
	case beacon.EventRequest:
		numeric = key == "http.response.status_code" || key == "duration_ns"
	}
	if numeric {
		value = beacon.Clean(value, len(value))
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return "", false
		}
		return beacon.Truncate(value, maxAttrLen), true
	}
	return o.cleanAttr(key, value), true
}
