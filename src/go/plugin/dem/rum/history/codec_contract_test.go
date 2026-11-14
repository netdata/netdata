// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// fullEventRecord sets every field to a distinct nonzero value, so a field added
// to the record without journal encoding and decoding fails the round trip.
func fullEventRecord(t *testing.T) EventRecord {
	t.Helper()
	var r EventRecord
	v := reflect.ValueOf(&r).Elem()
	for i := range v.NumField() {
		field, name := v.Field(i), v.Type().Field(i).Name
		switch field.Kind() {
		case reflect.String:
			field.SetString(name + "-value")
		case reflect.Uint64:
			field.SetUint(uint64(1000 + i))
		case reflect.Int64:
			field.SetInt(int64(1_700_000_000_000_000 + i))
		default:
			t.Fatalf("field %s has kind %s; extend fullEventRecord", name, field.Kind())
		}
	}
	r.VnodeGUID = "11111111-1111-4111-8111-111111111111"
	return r
}

// The journal field names and values are the persisted RUM history contract.
func TestEventFieldsContract(t *testing.T) {
	type field struct{ name, value string }
	full := fullEventRecord(t)
	tests := map[string]struct {
		record EventRecord
		want   []field
	}{
		"every field set": {
			record: full,
			want: []field{
				{"DEM_KIND", "rum"},
				{"DEM_OBSERVED_US", "1700000000000008"},
				{"DEM_REVISION", "1005"},
				{"DEM_EXPERIENCE_ID", "ExperienceID-value"},
				{"DEM_VIEW", "View-value"},
				{"DEM_VIEW_ID", "ViewID-value"},
				{"DEM_METRIC_ID", "MetricID-value"},
				{"DEM_VNODE_GUID", "11111111-1111-4111-8111-111111111111"},
				{"DEM_SITE", "Site-value"},
				{"DEM_SESSION_ID", "SessionID-value"},
				{"DEM_TYPE", "Type-value"},
				{"DEM_PAGE", "Page-value"},
				{"DEM_TEXT", "Text-value"},
				{"DEM_TRACE_ID", "TraceID-value"},
				{"DEM_BROWSER", "Browser-value"},
				{"DEM_DEVICE", "Device-value"},
				{"DEM_COUNTRY", "Country-value"},
				{"DEM_VERSION", "Version-value"},
				{"DEM_USER_ID", "UserID-value"},
				{"DEM_FINGERPRINT", "Fingerprint-value"},
				{"DEM_ERROR_TYPE", "ErrorType-value"},
				{"DEM_MESSAGE", "Message-value"},
				{"DEM_SAMPLE_STACK", "SampleStack-value"},
				{"MESSAGE", "Text-value"},
			},
		},
		"message falls back to error message": {
			record: EventRecord{
				Site:       "shop",
				Type:       "error",
				ObservedUS: 5,
				Message:    "failed",
			},
			want: []field{
				{"DEM_KIND", "rum"},
				{"DEM_OBSERVED_US", "5"},
				{"DEM_REVISION", "0"},
				{"DEM_SITE", "shop"},
				{"DEM_TYPE", "error"},
				{"DEM_MESSAGE", "failed"},
				{"MESSAGE", "failed"},
			},
		},
		"message falls back to type": {
			record: EventRecord{
				Site:       "shop",
				Type:       "activity",
				ObservedUS: 5,
			},
			want: []field{
				{"DEM_KIND", "rum"},
				{"DEM_OBSERVED_US", "5"},
				{"DEM_REVISION", "0"},
				{"DEM_SITE", "shop"},
				{"DEM_TYPE", "activity"},
				{"MESSAGE", "activity"},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got []field
			for _, f := range eventFields(tc.record) {
				got = append(got, field{f.Name, string(f.Value)})
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// Live and persisted timeline rows deduplicate by equality. Every row field must
// come from the same-named record field, so neither source can drift.
func TestSessionEventOfCarriesEveryRowField(t *testing.T) {
	record := fullEventRecord(t)
	row := reflect.ValueOf(SessionEventOf(record))
	source := reflect.ValueOf(record)
	for i := range row.NumField() {
		name := row.Type().Field(i).Name
		from := source.FieldByName(name)
		require.True(t, from.IsValid(), "row field %s has no record field", name)
		require.Equal(t, from.Interface(), row.Field(i).Interface(), "row field %s", name)
	}
}

func TestEventCodecRoundTripsEveryField(t *testing.T) {
	s, _ := newTestStore(t)
	want := fullEventRecord(t)
	appendEvent(t, s, want)
	var scanned, matched []EventRecord
	second := want.ObservedUS / 1_000_000
	require.NoError(t, s.scan(context.Background(), "", second, second, "", func(r EventRecord) {
		scanned = append(scanned, r)
	}))
	require.NoError(t, s.matchSession(context.Background(), "", want.SessionID, "", func(r EventRecord) {
		matched = append(matched, r)
	}))
	require.Equal(t, []EventRecord{want}, scanned)
	require.Equal(t, []EventRecord{want}, matched)
}
