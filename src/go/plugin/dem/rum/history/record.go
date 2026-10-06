// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"bytes"
	"fmt"
	"strconv"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// EventRecord is an immutable self-contained investigation event. TSUnixUS
// is original event time; the journal entry realtime records when it was saved.
type EventRecord struct {
	ExperienceID, View, ViewID, MetricID         string
	Revision                                     uint64
	Site, SessionID                              string
	TSUnixUS                                     int64
	Type, Page, Text, TraceID                    string
	Browser, Device, Country, Version, UserID    string
	Fingerprint, ErrorType, Message, SampleStack string
}

// SessionRecord summarizes only retained events selected by the saved-time
// filter. Timestamps and page/metadata ordering use original event time.
// UserIDs are the sorted distinct nonempty IDs on those events, not session ownership.
type SessionRecord struct {
	Site, SessionID                     string
	StartedAt, LastAt                   int64
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
	TSUnixUS                             int64
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
		journal.StringField("DEM_TS_US", strconv.FormatInt(r.TSUnixUS, 10)),
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

// The SDK's GetEntry materializes several complete field representations.
// Read callback-scoped payloads directly and retain only domain strings.
func decodeEvent(reader *demjournal.Snapshot) (EventRecord, bool, error) {
	var r EventRecord
	var kind, timestamp, revision string
	err := reader.VisitEntryPayloads(func(payload []byte) error {
		separator := bytes.IndexByte(payload, '=')
		if separator < 0 {
			return fmt.Errorf("invalid journal field payload")
		}
		value := payload[separator+1:]
		switch string(payload[:separator]) {
		case "DEM_KIND":
			kind = string(value)
		case "DEM_TS_US":
			timestamp = string(value)
		case "DEM_REVISION":
			revision = string(value)
		case "DEM_EXPERIENCE_ID":
			r.ExperienceID = string(value)
		case "DEM_VIEW":
			r.View = string(value)
		case "DEM_VIEW_ID":
			r.ViewID = string(value)
		case "DEM_METRIC_ID":
			r.MetricID = string(value)
		case "DEM_SITE":
			r.Site = string(value)
		case "DEM_SESSION_ID":
			r.SessionID = string(value)
		case "DEM_TYPE":
			r.Type = string(value)
		case "DEM_PAGE":
			r.Page = string(value)
		case "DEM_TEXT":
			r.Text = string(value)
		case "DEM_TRACE_ID":
			r.TraceID = string(value)
		case "DEM_BROWSER":
			r.Browser = string(value)
		case "DEM_DEVICE":
			r.Device = string(value)
		case "DEM_COUNTRY":
			r.Country = string(value)
		case "DEM_VERSION":
			r.Version = string(value)
		case "DEM_USER_ID":
			r.UserID = string(value)
		case "DEM_FINGERPRINT":
			r.Fingerprint = string(value)
		case "DEM_ERROR_TYPE":
			r.ErrorType = string(value)
		case "DEM_MESSAGE":
			r.Message = string(value)
		case "DEM_SAMPLE_STACK":
			r.SampleStack = string(value)
		}
		return nil
	})
	if err != nil || kind != "rum" {
		return r, false, err
	}
	if revision != "" {
		r.Revision, err = strconv.ParseUint(revision, 10, 64)
		if err != nil {
			return r, false, fmt.Errorf("invalid DEM_REVISION: %w", err)
		}
	}
	r.TSUnixUS, err = strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return r, false, fmt.Errorf("invalid DEM_TS_US: %w", err)
	}
	return r, true, nil
}
