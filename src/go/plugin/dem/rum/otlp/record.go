// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// maxStackBytes bounds error.stack (≤4 KiB). the Faro decoder already
// truncates at this length before the beacon reaches here; this is a
// defensive second cap so a future producer cannot regress it silently.
const maxStackBytes = 4096

// queued is one record waiting on the owning site's export queue.
type queued struct{ rec *logspb.LogRecord }

// build turns one accepted beacon into zero or more queued records:
// pageview, session_start (first sighting per session/TTL), one per JS
// error, one per custom event, one per console log line (already
// filtered upstream by collect_console_logs). Free-form text is redacted
// before it reaches the record.
func (e *Exporter) build(b *beacon.Beacon, pageView bool) []queued {
	base := baseAttrs(b)
	now := e.now()
	var out []queued

	if b.SessionID != "" && e.sessions.starts(b.SessionID, now) {
		out = append(out, e.record(base, "session_start", b.Received, sevInfo, "session start"))
	}
	if pageView {
		out = append(out, e.record(base, "pageview", b.Received, sevInfo, "pageview "+b.Path))
	}
	for _, err := range b.Errors {
		msg := e.redact(err.Message)
		stack := truncate(e.redact(err.Stack), maxStackBytes)
		attrs := append(append([]*commonpb.KeyValue{}, base...),
			strAttr("error.type", err.Type), strAttr("error.message", msg), strAttr("error.stack", stack))
		if err.Fingerprint != "" {
			attrs = append(attrs, strAttr("error.fingerprint", err.Fingerprint))
		}
		body := err.Type + ": " + msg + " @ " + b.Path
		out = append(out, e.record(attrs, "error", err.Time, sevError, body))
	}
	for _, ev := range b.Events {
		attrs := append(append([]*commonpb.KeyValue{}, base...), strAttr("event.name", ev.Name))
		for k, v := range ev.Attrs {
			attrs = append(attrs, strAttr("event.attr."+k, e.redact(v)))
		}
		body := "event " + ev.Name + " @ " + b.Path
		out = append(out, e.record(attrs, "event", ev.Time, sevInfo, body))
	}
	for _, l := range b.Logs {
		msg := e.redact(l.Message)
		body := "console " + l.Level + ": " + msg
		out = append(out, e.record(base, "console", l.Time, sevInfo, body))
	}
	return out
}

const (
	sevInfo  = logspb.SeverityNumber_SEVERITY_NUMBER_INFO
	sevError = logspb.SeverityNumber_SEVERITY_NUMBER_ERROR
)

// baseAttrs are the record attributes every RUM event type carries,
// skipping empty values.
func baseAttrs(b *beacon.Beacon) []*commonpb.KeyValue {
	var out []*commonpb.KeyValue
	add := func(k, v string) {
		if v != "" {
			out = append(out, strAttr(k, v))
		}
	}
	add("session.id", b.SessionID)
	add("page.path", b.Path)
	add("page.group", b.PageGroup)
	add("page.view", b.View)
	add("browser.name", b.Browser)
	add("browser.version", b.BrowserVersion)
	add("os", b.OS)
	add("device", b.Device)
	add("country", b.Country)
	add("app.version", b.AppVersion)
	add("app.environment", b.Environment)
	return out
}

func (e *Exporter) record(
	attrs []*commonpb.KeyValue,
	typ string,
	ts time.Time,
	sev logspb.SeverityNumber,
	body string,
) queued {
	if ts.IsZero() {
		ts = e.now()
	}
	all := append([]*commonpb.KeyValue{strAttr("rum.type", typ)}, attrs...)
	rec := &logspb.LogRecord{
		TimeUnixNano:         uint64(ts.UnixNano()),
		ObservedTimeUnixNano: uint64(e.now().UnixNano()),
		SeverityNumber:       sev,
		SeverityText:         sevText(sev),
		Body: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{
				StringValue: body,
			},
		},
		Attributes: all,
	}
	return queued{
		rec: rec,
	}
}

func sevText(sev logspb.SeverityNumber) string {
	if sev == sevError {
		return "ERROR"
	}
	return "INFO"
}

func strAttr(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: k,
		Value: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{
				StringValue: v,
			},
		},
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n-- // never cut inside a multi-byte rune
	}
	return s[:n]
}
