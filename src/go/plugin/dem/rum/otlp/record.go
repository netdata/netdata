// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// queued is one record waiting on the owning site's export queue.
type queued struct{ rec *logspb.LogRecord }

// build turns admitted observations into document activations, vital updates,
// JS errors, custom events and console log lines (already
// filtered upstream by ConsoleLogsOn). Receiver normalization owns all text
// cleaning and bounds; export preserves the accepted observation.
func (e *Logs) build(b *beacon.Beacon, pageView bool) []queued {
	base := baseAttrs(b)
	var out []queued

	if pageView {
		attrs, ts := base, b.Received
		for _, event := range b.Events {
			if event.Kind == beacon.EventDocument {
				attrs = append(append([]*commonpb.KeyValue{}, base...), strAttr("event.id", event.ID), strAttr("event.revision", strconv.FormatUint(event.Revision, 10)))
				if !event.Time.IsZero() {
					ts = event.Time
				}
				break
			}
		}
		out = append(out, e.record(attrs, "pageview", ts, sevInfo, "pageview "+b.PageGroup))
	}
	for _, vital := range b.Vitals {
		vitalBase := base
		page := b.PageGroup
		if origin := vital.Origin; origin != nil {
			observed := *b
			observed.PageGroup, observed.Browser, observed.Device = origin.PageGroup, origin.Browser, origin.Device
			observed.Country, observed.AppVersion = origin.Country, origin.AppVersion
			vitalBase, page = baseAttrs(&observed), origin.PageGroup
		}
		attrs := append(append([]*commonpb.KeyValue{}, vitalBase...),
			strAttr("metric.name", vital.Name), strAttr("metric.id", vital.ID),
			strAttr("metric.revision", strconv.FormatUint(vital.Revision, 10)),
			strAttr("metric.value", strconv.FormatFloat(vital.Value, 'g', -1, 64)),
			strAttr("metric.element", vital.Element))
		out = append(out, e.record(attrs, "vital", vital.Time, sevInfo, "vital "+vital.Name+" @ "+page))
	}
	for _, err := range b.Errors {
		msg := err.Message
		stack := err.Stack
		attrs := append(append([]*commonpb.KeyValue{}, base...),
			strAttr("error.type", err.Type), strAttr("error.message", msg), strAttr("error.stack", stack))
		if err.Fingerprint != "" {
			attrs = append(attrs, strAttr("error.fingerprint", err.Fingerprint))
		}
		body := err.Type + ": " + msg + " @ " + b.Path
		out = append(out, e.record(attrs, "error", err.Time, sevError, body))
	}
	for _, ev := range b.Events {
		if ev.Kind == beacon.EventSession || ev.Kind == beacon.EventDocument {
			continue
		}
		attrs := append(append([]*commonpb.KeyValue{}, base...), strAttr("event.name", ev.Name))
		if ev.ID != "" {
			attrs = append(attrs, strAttr("event.id", ev.ID), strAttr("event.revision", strconv.FormatUint(ev.Revision, 10)))
		}
		for k, v := range ev.Attrs {
			attrs = append(attrs, strAttr("event.attr."+k, v))
		}
		body := "event " + ev.Name + " @ " + b.Path
		out = append(out, e.record(attrs, "event", ev.Time, sevInfo, body))
	}
	for _, l := range b.Logs {
		msg := l.Message
		body := "console " + l.Level + ": " + msg
		attrs := append(append([]*commonpb.KeyValue{}, base...), strAttr("console.level", l.Level))
		if typ := l.ErrorType; typ != "" {
			attrs = append(attrs, strAttr("error.type", typ))
		}
		if stack := l.Stack; stack != "" {
			attrs = append(attrs, strAttr("error.stack", stack))
		}
		out = append(out, e.record(attrs, "console", l.Time, consoleSeverity(l.Level), body))
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
	add("document.experience.id", b.ExperienceID)
	add("application.view.id", b.ViewID)
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

func (e *Logs) record(
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

func consoleSeverity(level string) logspb.SeverityNumber {
	switch strings.ToLower(level) {
	case "error":
		return sevError
	case "warn", "warning":
		return logspb.SeverityNumber_SEVERITY_NUMBER_WARN
	case "debug":
		return logspb.SeverityNumber_SEVERITY_NUMBER_DEBUG
	case "trace":
		return logspb.SeverityNumber_SEVERITY_NUMBER_TRACE
	default:
		return sevInfo
	}
}
func sevText(sev logspb.SeverityNumber) string {
	return strings.TrimPrefix(sev.String(), "SEVERITY_NUMBER_")
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
