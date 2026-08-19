package main

import (
	"testing"
	"time"

	logapi "go.opentelemetry.io/otel/log"
)

func TestSeverityFromPriority(t *testing.T) {
	tests := map[string]struct {
		input    interface{}
		wantSev  logapi.Severity
		wantText string
		wantOK   bool
	}{
		"emergency":         {float64(0), logapi.SeverityFatal, "emergency", true},
		"alert":             {float64(1), logapi.SeverityError3, "alert", true},
		"critical":          {float64(2), logapi.SeverityError2, "critical", true},
		"error":             {float64(3), logapi.SeverityError, "error", true},
		"warning":           {float64(4), logapi.SeverityWarn, "warning", true},
		"notice":            {float64(5), logapi.SeverityInfo2, "notice", true},
		"info":              {float64(6), logapi.SeverityInfo, "info", true},
		"debug":             {float64(7), logapi.SeverityDebug, "debug", true},
		"missing":           {nil, logapi.SeverityUndefined, "", false},
		"string value":      {"3", logapi.SeverityUndefined, "", false},
		"fractional number": {3.5, logapi.SeverityUndefined, "", false},
		"out of range":      {float64(8), logapi.SeverityUndefined, "", false},
		"negative":          {float64(-1), logapi.SeverityUndefined, "", false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sev, text, ok := severityFromPriority(tc.input)
			if sev != tc.wantSev || text != tc.wantText || ok != tc.wantOK {
				t.Errorf("severityFromPriority(%v) = (%v, %q, %v), want (%v, %q, %v)",
					tc.input, sev, text, ok, tc.wantSev, tc.wantText, tc.wantOK)
			}
		})
	}
}

func TestBuildRecord(t *testing.T) {
	ts := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	payload := []byte(`{"exit_cause":"deadly signal","priority":2,"agent":{"id":"test"}}`)

	tests := map[string]struct {
		event    map[string]interface{}
		wantSev  logapi.Severity
		wantText string
	}{
		"with priority":      {map[string]interface{}{"priority": float64(2)}, logapi.SeverityError2, "critical"},
		"without priority":   {map[string]interface{}{}, logapi.SeverityUndefined, ""},
		"malformed priority": {map[string]interface{}{"priority": "high"}, logapi.SeverityUndefined, ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := buildRecord(ts, tc.event, payload)
			if !rec.Timestamp().Equal(ts) {
				t.Errorf("Timestamp = %v, want %v", rec.Timestamp(), ts)
			}
			if !rec.ObservedTimestamp().Equal(ts) {
				t.Errorf("ObservedTimestamp = %v, want %v", rec.ObservedTimestamp(), ts)
			}
			if got := rec.Body().AsString(); got != string(payload) {
				t.Errorf("Body = %q, want %q", got, string(payload))
			}
			if rec.Severity() != tc.wantSev {
				t.Errorf("Severity = %v, want %v", rec.Severity(), tc.wantSev)
			}
			if rec.SeverityText() != tc.wantText {
				t.Errorf("SeverityText = %q, want %q", rec.SeverityText(), tc.wantText)
			}
		})
	}
}
