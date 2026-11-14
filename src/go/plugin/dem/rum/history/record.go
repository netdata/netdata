// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"bytes"
	"fmt"
	"strconv"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// EventRecord names the producer's record in history terms. It is the same type,
// so the producer, writer and journal codec share one field list.
type EventRecord = aggregate.HistoryEvent

// SessionRecord summarizes retained observations selected by Agent receipt time.
// StartedAt and LastAt are seconds; LastObservedUS preserves ordering precision.
// UserIDs are the sorted distinct nonempty IDs on those events, not session ownership.
type SessionRecord struct {
	VnodeGUID                           string
	Site, SessionID                     string
	StartedAt, LastAt                   int64
	LastObservedUS                      int64
	Pageviews, ApplicationViews, Errors int64
	Browser, Device, Country, Version   string
	EntryPage, LastPage                 string
	UserIDs                             []string
	Frustrations                        int64
}

// SessionEventRecord is one rum-session-events row.
type SessionEventRecord struct {
	VnodeGUID                            string
	ExperienceID, View, ViewID, MetricID string
	Revision                             uint64
	Site, SessionID                      string
	ObservedUS                           int64
	Type, Page, Text, TraceID, UserID    string
}

// SessionEventOf projects a record onto its timeline row. Live and persisted rows
// both use it, so the same event compares equal when the two sources overlap.
func SessionEventOf(r EventRecord) SessionEventRecord {
	return SessionEventRecord{
		VnodeGUID:    r.VnodeGUID,
		ExperienceID: r.ExperienceID,
		View:         r.View,
		ViewID:       r.ViewID,
		MetricID:     r.MetricID,
		Revision:     r.Revision,
		Site:         r.Site,
		SessionID:    r.SessionID,
		ObservedUS:   r.ObservedUS,
		Type:         r.Type,
		Page:         r.Page,
		Text:         r.Text,
		TraceID:      r.TraceID,
		UserID:       r.UserID,
	}
}

// ErrorGroup counts the selected retained errors. Details is true only for a
// selected fingerprint; otherwise SessionsAffected, TopPage and Browsers are
// unset and must be displayed as unavailable, not as measured zeroes.
type ErrorGroup struct {
	VnodeGUID                                     string
	Site, Fingerprint, Type, Message, SampleStack string
	CountWindow, SessionsAffected                 int
	FirstSeen, LastSeen                           int64
	TopPage                                       string
	Browsers                                      []string
	Details                                       bool
}

func eventFields(r EventRecord) []journal.Field {
	fields := []journal.Field{
		journal.StringField("DEM_KIND", "rum"),
		journal.StringField("DEM_OBSERVED_US", strconv.FormatInt(r.ObservedUS, 10)),
		journal.StringField("DEM_REVISION", strconv.FormatUint(r.Revision, 10)),
	}
	for _, field := range []struct{ name, value string }{
		{"DEM_EXPERIENCE_ID", r.ExperienceID}, {"DEM_VIEW", r.View}, {"DEM_VIEW_ID", r.ViewID}, {"DEM_METRIC_ID", r.MetricID},
		{"DEM_VNODE_GUID", r.VnodeGUID}, {"DEM_SITE", r.Site}, {"DEM_SESSION_ID", r.SessionID}, {"DEM_TYPE", r.Type},
		{"DEM_PAGE", r.Page}, {"DEM_TEXT", r.Text}, {"DEM_TRACE_ID", r.TraceID},
		{"DEM_BROWSER", r.Browser}, {"DEM_DEVICE", r.Device}, {"DEM_COUNTRY", r.Country},
		{"DEM_VERSION", r.Version}, {"DEM_USER_ID", r.UserID},
		{"DEM_FINGERPRINT", r.Fingerprint}, {"DEM_ERROR_TYPE", r.ErrorType},
		{"DEM_MESSAGE", r.Message}, {"DEM_SAMPLE_STACK", r.SampleStack},
	} {
		if field.value != "" {
			fields = append(fields, journal.StringField(field.name, field.value))
		}
	}
	message := r.Text
	if message == "" {
		message = r.Message
	}
	if message == "" {
		message = r.Type
	}
	return append(fields, journal.StringField("MESSAGE", message))
}

// Read callback-scoped payloads directly and retain only domain strings.
// The prior record owns its strings; equal values can reuse them across callbacks.
func decodeEvent(entry *journal.SnapshotEntry, previous *EventRecord) (EventRecord, bool, error) {
	var r EventRecord
	var envelope demjournal.Envelope
	var revision string
	var seen uint32
	err := entry.VisitPayloads(func(payload []byte) error {
		separator := bytes.IndexByte(payload, '=')
		if separator < 0 {
			return fmt.Errorf("invalid journal field payload")
		}
		name, value := payload[:separator], payload[separator+1:]
		var bit uint32
		switch string(name) {
		case "DEM_REVISION":
			bit = 1 << 0
			revision = string(value)
		case "DEM_EXPERIENCE_ID":
			bit = 1 << 1
			r.ExperienceID = reuseEventString(value, previous.ExperienceID)
		case "DEM_VIEW":
			bit = 1 << 2
			r.View = reuseEventString(value, previous.View)
		case "DEM_VIEW_ID":
			bit = 1 << 3
			r.ViewID = reuseEventString(value, previous.ViewID)
		case "DEM_METRIC_ID":
			bit = 1 << 4
			r.MetricID = reuseEventString(value, previous.MetricID)
		case "DEM_VNODE_GUID":
			bit = 1 << 20
			r.VnodeGUID = reuseEventString(value, previous.VnodeGUID)
		case "DEM_SITE":
			bit = 1 << 5
			r.Site = reuseEventString(value, previous.Site)
		case "DEM_SESSION_ID":
			bit = 1 << 6
			r.SessionID = reuseEventString(value, previous.SessionID)
		case "DEM_TYPE":
			bit = 1 << 7
			r.Type = reuseEventString(value, previous.Type)
		case "DEM_PAGE":
			bit = 1 << 8
			r.Page = reuseEventString(value, previous.Page)
		case "DEM_TEXT":
			bit = 1 << 9
			r.Text = reuseEventString(value, previous.Text)
		case "DEM_TRACE_ID":
			bit = 1 << 10
			r.TraceID = reuseEventString(value, previous.TraceID)
		case "DEM_BROWSER":
			bit = 1 << 11
			r.Browser = reuseEventString(value, previous.Browser)
		case "DEM_DEVICE":
			bit = 1 << 12
			r.Device = reuseEventString(value, previous.Device)
		case "DEM_COUNTRY":
			bit = 1 << 13
			r.Country = reuseEventString(value, previous.Country)
		case "DEM_VERSION":
			bit = 1 << 14
			r.Version = reuseEventString(value, previous.Version)
		case "DEM_USER_ID":
			bit = 1 << 15
			r.UserID = reuseEventString(value, previous.UserID)
		case "DEM_FINGERPRINT":
			bit = 1 << 16
			r.Fingerprint = reuseEventString(value, previous.Fingerprint)
		case "DEM_ERROR_TYPE":
			bit = 1 << 17
			r.ErrorType = reuseEventString(value, previous.ErrorType)
		case "DEM_MESSAGE":
			bit = 1 << 18
			r.Message = reuseEventString(value, previous.Message)
		case "DEM_SAMPLE_STACK":
			bit = 1 << 19
			r.SampleStack = reuseEventString(value, previous.SampleStack)
		default:
			return envelope.Observe(name, value)
		}
		if seen&bit != 0 {
			return fmt.Errorf("duplicate %s", name)
		}
		seen |= bit
		return nil
	})
	if err != nil {
		return r, false, err
	}
	kind, observed, err := envelope.Validate()
	if err != nil || kind != "rum" {
		return r, false, err
	}
	r.ObservedUS = observed
	r.Revision, err = strconv.ParseUint(revision, 10, 64)
	if err != nil || strconv.FormatUint(r.Revision, 10) != revision {
		return r, false, fmt.Errorf("invalid DEM_REVISION %q", revision)
	}
	if err := validateEvent(r); err != nil {
		return r, false, err
	}
	return r, true, nil
}

func validateEvent(r EventRecord) error {
	if r.Site == "" || r.Type == "" {
		return fmt.Errorf("RUM history requires site and event type")
	}
	if r.ObservedUS < 0 {
		return fmt.Errorf("RUM observation time must not precede epoch")
	}
	return nil
}

// Only the previous event is retained, so reuse needs no growing intern table.
func reuseEventString(value []byte, previous string) string {
	// Reject different final bytes before comparing longer suffixes or shared prefixes.
	if len(value) == len(previous) &&
		(len(value) == 0 || value[len(value)-1] == previous[len(previous)-1]) &&
		(len(value) <= 8 || string(value[len(value)-8:]) == previous[len(previous)-8:]) &&
		string(value) == previous {
		return previous
	}
	return string(value)
}
