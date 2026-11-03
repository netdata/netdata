// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"bytes"
	"fmt"
	"strconv"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// EventRecord is an immutable self-contained investigation event. ObservedUS
// is Agent receipt time, preserved through delayed promotion and persistence.
type EventRecord struct {
	ExperienceID, View, ViewID, MetricID         string
	Revision                                     uint64
	Site, SessionID                              string
	ObservedUS                                   int64
	Type, Page, Text, TraceID                    string
	Browser, Device, Country, Version, UserID    string
	Fingerprint, ErrorType, Message, SampleStack string
}

// SessionRecord summarizes retained observations selected by Agent receipt time.
// StartedAt and LastAt are seconds; LastObservedUS preserves ordering precision.
// UserIDs are the sorted distinct nonempty IDs on those events, not session ownership.
type SessionRecord struct {
	Site, SessionID                     string
	StartedAt, LastAt                   int64
	LastObservedUS                      int64
	Pageviews, ApplicationViews, Errors int64
	Browser, Device, Country, Version   string
	EntryPage, LastPage                 string
	UserIDs                             []string
	Frustrations                        int64
}

type SessionEventRecord struct {
	ExperienceID, View, ViewID, MetricID string
	Revision                             uint64
	Site, SessionID                      string
	ObservedUS                           int64
	Type, Page, Text, TraceID, UserID    string
}

// ErrorGroup counts the selected retained errors. Details is true only for a
// selected fingerprint; otherwise SessionsAffected, TopPage and Browsers are
// unset and must be displayed as unavailable, not as measured zeroes.
type ErrorGroup struct {
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
		{"DEM_SITE", r.Site}, {"DEM_SESSION_ID", r.SessionID}, {"DEM_TYPE", r.Type},
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
func decodeEvent(entry *journal.SnapshotEntry) (EventRecord, bool, error) {
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
			r.ExperienceID = string(value)
		case "DEM_VIEW":
			bit = 1 << 2
			r.View = string(value)
		case "DEM_VIEW_ID":
			bit = 1 << 3
			r.ViewID = string(value)
		case "DEM_METRIC_ID":
			bit = 1 << 4
			r.MetricID = string(value)
		case "DEM_SITE":
			bit = 1 << 5
			r.Site = string(value)
		case "DEM_SESSION_ID":
			bit = 1 << 6
			r.SessionID = string(value)
		case "DEM_TYPE":
			bit = 1 << 7
			r.Type = string(value)
		case "DEM_PAGE":
			bit = 1 << 8
			r.Page = string(value)
		case "DEM_TEXT":
			bit = 1 << 9
			r.Text = string(value)
		case "DEM_TRACE_ID":
			bit = 1 << 10
			r.TraceID = string(value)
		case "DEM_BROWSER":
			bit = 1 << 11
			r.Browser = string(value)
		case "DEM_DEVICE":
			bit = 1 << 12
			r.Device = string(value)
		case "DEM_COUNTRY":
			bit = 1 << 13
			r.Country = string(value)
		case "DEM_VERSION":
			bit = 1 << 14
			r.Version = string(value)
		case "DEM_USER_ID":
			bit = 1 << 15
			r.UserID = string(value)
		case "DEM_FINGERPRINT":
			bit = 1 << 16
			r.Fingerprint = string(value)
		case "DEM_ERROR_TYPE":
			bit = 1 << 17
			r.ErrorType = string(value)
		case "DEM_MESSAGE":
			bit = 1 << 18
			r.Message = string(value)
		case "DEM_SAMPLE_STACK":
			bit = 1 << 19
			r.SampleStack = string(value)
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
