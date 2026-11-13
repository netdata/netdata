// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodedEventsOwnStringsAndDoNotInheritAbsentFields(t *testing.T) {
	s, _ := newTestStore(t)
	full := EventRecord{
		VnodeGUID:    "11111111-1111-4111-8111-111111111111",
		ExperienceID: "document",
		View:         "view",
		ViewID:       "view-id",
		MetricID:     "metric",
		Revision:     2,
		Site:         "site",
		SessionID:    "session",
		ObservedUS:   1_000_000,
		Type:         "error",
		Page:         "/page",
		Text:         "event-1-with-a-shared-tail",
		TraceID:      "trace",
		Browser:      "browser",
		Device:       "device",
		Country:      "country",
		Version:      "version",
		UserID:       "user",
		Fingerprint:  "fingerprint",
		ErrorType:    "error-type",
		Message:      "message1",
		SampleStack:  "frame-line-A!",
	}
	changed := EventRecord{
		VnodeGUID:    "22222222-2222-4222-8222-222222222222",
		ExperienceID: "other-document",
		View:         "other-view",
		ViewID:       "other-view-id",
		MetricID:     "other-metric",
		Revision:     3,
		Site:         "other-site",
		SessionID:    "other-session",
		ObservedUS:   2_000_000,
		Type:         "event",
		Page:         "/other-page",
		Text:         "event-2-with-a-shared-tail",
		TraceID:      "other-trace",
		Browser:      "other-browser",
		Device:       "other-device",
		Country:      "other-country",
		Version:      "other-version",
		UserID:       "other-user",
		Fingerprint:  "other-fingerprint",
		ErrorType:    "other-error-type",
		Message:      "message2",
		SampleStack:  "frame-line-B!",
	}
	want := []EventRecord{full, full, changed, {Site: "empty", Type: "activity", ObservedUS: 3_000_000}, full}
	for _, r := range want {
		appendEvent(t, s, r)
	}
	var got []EventRecord
	require.NoError(t, s.scan(context.Background(), "", 0, 4, "", func(r EventRecord) { got = append(got, r) }))
	require.Equal(t, want, got, "records remain owned after later payload callbacks and reader close")
	var matched []EventRecord
	require.NoError(t, s.matchSession(context.Background(), "", "session", "", func(r EventRecord) { matched = append(matched, r) }))
	require.Equal(t, []EventRecord{full, full, full}, matched)
}
