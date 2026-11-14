// SPDX-License-Identifier: GPL-3.0-or-later

package journalstore

import (
	"testing"

	"github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/stretchr/testify/assert"
)

type envelopeField struct{ name, value string }

// The envelope names, values and error texts are the persisted history contract:
// the same retained bytes must decode to the same kind and clock, or fail the same way.
func TestEnvelopeDecodeContract(t *testing.T) {
	type result struct {
		Kind      string
		Timestamp int64
		Err       string
	}
	rum := []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "60000001"}, {"DEM_RUM_MINUTE", "1"}}
	synthetic := []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "synthetic"}, {"DEM_STARTED_US", "120000001"}, {"DEM_SYNTHETIC_MINUTE", "2"}}
	with := func(base []envelopeField, extra ...envelopeField) []envelopeField {
		return append(append([]envelopeField(nil), base...), extra...)
	}
	tests := map[string]struct {
		fields []envelopeField
		want   result
	}{
		"rum": {
			fields: rum,
			want:   result{Kind: "rum", Timestamp: 60000001},
		},
		"synthetic": {
			fields: synthetic,
			want:   result{Kind: "synthetic", Timestamp: 120000001},
		},
		"rum at epoch": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "0"}, {"DEM_RUM_MINUTE", "0"}},
			want:   result{Kind: "rum", Timestamp: 0},
		},
		"any field order": {
			fields: []envelopeField{{"DEM_RUM_MINUTE", "1"}, {"DEM_OBSERVED_US", "60000001"}, {"DEM_KIND", "rum"}, {"DEM_SCHEMA", "1"}},
			want:   result{Kind: "rum", Timestamp: 60000001},
		},
		"domain fields ignored": {
			fields: with(rum, envelopeField{"DEM_SITE", "shop"}, envelopeField{"MESSAGE", "m"}, envelopeField{"DEM_SITE", "again"}),
			want:   result{Kind: "rum", Timestamp: 60000001},
		},
		"synthetic at epoch": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "synthetic"}, {"DEM_STARTED_US", "0"}, {"DEM_SYNTHETIC_MINUTE", "0"}},
			want:   result{Err: "invalid synthetic history clock/schema envelope"},
		},
		"unsupported schema": {
			fields: []envelopeField{{"DEM_SCHEMA", "2"}},
			want:   result{Err: `incompatible DEM history schema "2"`},
		},
		"invalid kind": {
			fields: []envelopeField{{"DEM_KIND", "other"}},
			want:   result{Err: `invalid DEM_KIND "other"`},
		},
		"duplicate schema": {
			fields: with(rum, envelopeField{"DEM_SCHEMA", "1"}),
			want:   result{Err: "duplicate history field DEM_SCHEMA"},
		},
		"duplicate kind": {
			fields: with(rum, envelopeField{"DEM_KIND", "rum"}),
			want:   result{Err: "duplicate history field DEM_KIND"},
		},
		"duplicate kind with another value": {
			fields: with(rum, envelopeField{"DEM_KIND", "synthetic"}),
			want:   result{Err: "duplicate history field DEM_KIND"},
		},
		"duplicate clock": {
			fields: with(synthetic, envelopeField{"DEM_STARTED_US", "120000001"}),
			want:   result{Err: "duplicate history field DEM_STARTED_US"},
		},
		"duplicate bucket": {
			fields: with(rum, envelopeField{"DEM_RUM_MINUTE", "1"}),
			want:   result{Err: "duplicate history field DEM_RUM_MINUTE"},
		},
		"noncanonical clock": {
			fields: []envelopeField{{"DEM_OBSERVED_US", "0123"}},
			want:   result{Err: "invalid DEM_OBSERVED_US: expected canonical nonnegative decimal"},
		},
		"negative clock": {
			fields: []envelopeField{{"DEM_STARTED_US", "-1"}},
			want:   result{Err: "invalid DEM_STARTED_US: expected canonical nonnegative decimal"},
		},
		"empty bucket": {
			fields: []envelopeField{{"DEM_SYNTHETIC_MINUTE", ""}},
			want:   result{Err: "invalid DEM_SYNTHETIC_MINUTE: expected canonical nonnegative decimal"},
		},
		"noncanonical bucket": {
			fields: []envelopeField{{"DEM_RUM_MINUTE", "01"}},
			want:   result{Err: "invalid DEM_RUM_MINUTE: expected canonical nonnegative decimal"},
		},
		"clock out of range": {
			fields: []envelopeField{{"DEM_OBSERVED_US", "99999999999999999999"}},
			want:   result{Err: `invalid DEM_OBSERVED_US: strconv.ParseInt: parsing "99999999999999999999": value out of range`},
		},
		"missing kind": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_OBSERVED_US", "60000001"}, {"DEM_RUM_MINUTE", "1"}},
			want:   result{Err: `invalid DEM_KIND ""`},
		},
		"missing schema": {
			fields: rum[1:],
			want:   result{Err: "invalid rum history clock/schema envelope"},
		},
		"missing bucket": {
			fields: synthetic[:3],
			want:   result{Err: "invalid synthetic history clock/schema envelope"},
		},
		"missing clock": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "rum"}, {"DEM_RUM_MINUTE", "0"}},
			want:   result{Err: "invalid rum history clock/schema envelope"},
		},
		"rum with start clock": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "rum"}, {"DEM_STARTED_US", "60000001"}, {"DEM_RUM_MINUTE", "1"}},
			want:   result{Err: "invalid rum history clock/schema envelope"},
		},
		"synthetic with observation clock": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "synthetic"}, {"DEM_OBSERVED_US", "120000001"}, {"DEM_SYNTHETIC_MINUTE", "2"}},
			want:   result{Err: "invalid synthetic history clock/schema envelope"},
		},
		"rum with synthetic bucket": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "60000001"}, {"DEM_SYNTHETIC_MINUTE", "1"}},
			want:   result{Err: "invalid rum history clock/schema envelope"},
		},
		"both clocks": {
			fields: with(rum, envelopeField{"DEM_STARTED_US", "60000001"}),
			want:   result{Err: "invalid rum history clock/schema envelope"},
		},
		"both buckets": {
			fields: with(synthetic, envelopeField{"DEM_RUM_MINUTE", "2"}),
			want:   result{Err: "invalid synthetic history clock/schema envelope"},
		},
		"bucket disagrees with clock": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "60000001"}, {"DEM_RUM_MINUTE", "2"}},
			want:   result{Err: "invalid rum history clock/schema envelope"},
		},
		"bucket is the floor minute": {
			fields: []envelopeField{{"DEM_SCHEMA", "1"}, {"DEM_KIND", "synthetic"}, {"DEM_STARTED_US", "119999999"}, {"DEM_SYNTHETIC_MINUTE", "1"}},
			want:   result{Kind: "synthetic", Timestamp: 119999999},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var envelope Envelope
			var got result
			for _, field := range tc.fields {
				if err := envelope.Observe([]byte(field.name), []byte(field.value)); err != nil {
					got.Err = err.Error()
					break
				}
			}
			if got.Err == "" {
				kind, timestamp, err := envelope.Validate()
				got = result{Kind: string(kind), Timestamp: timestamp}
				if err != nil {
					got.Err = err.Error()
				}
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// The append boundary derives the stored schema and minute bucket; callers
// declare only the kind and its clock.
func TestAppendEnvelopeContract(t *testing.T) {
	type result struct {
		Fields []envelopeField
		Kind   string
		Err    string
	}
	tests := map[string]struct {
		fields []envelopeField
		want   result
	}{
		"rum": {
			fields: []envelopeField{{"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "60000001"}, {"MESSAGE", "m"}},
			want: result{
				Fields: []envelopeField{{"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "60000001"}, {"MESSAGE", "m"}, {"DEM_SCHEMA", "1"}, {"DEM_RUM_MINUTE", "1"}},
				Kind:   "rum",
			},
		},
		"rum at epoch": {
			fields: []envelopeField{{"DEM_OBSERVED_US", "0"}, {"DEM_KIND", "rum"}},
			want: result{
				Fields: []envelopeField{{"DEM_OBSERVED_US", "0"}, {"DEM_KIND", "rum"}, {"DEM_SCHEMA", "1"}, {"DEM_RUM_MINUTE", "0"}},
				Kind:   "rum",
			},
		},
		"synthetic": {
			fields: []envelopeField{{"DEM_KIND", "synthetic"}, {"DEM_RUN_ID", "r"}, {"DEM_STARTED_US", "1700000000000008"}},
			want: result{
				Fields: []envelopeField{{"DEM_KIND", "synthetic"}, {"DEM_RUN_ID", "r"}, {"DEM_STARTED_US", "1700000000000008"}, {"DEM_SCHEMA", "1"}, {"DEM_SYNTHETIC_MINUTE", "28333333"}},
				Kind:   "synthetic",
			},
		},
		"synthetic at epoch": {
			fields: []envelopeField{{"DEM_KIND", "synthetic"}, {"DEM_STARTED_US", "0"}},
			want:   result{Err: "synthetic history requires exactly one positive start timestamp"},
		},
		"missing kind": {
			fields: []envelopeField{{"DEM_OBSERVED_US", "5"}, {"MESSAGE", "m"}},
			want:   result{Err: `invalid DEM_KIND ""`},
		},
		"invalid kind": {
			fields: []envelopeField{{"DEM_KIND", "other"}, {"DEM_SCHEMA", "1"}},
			want:   result{Err: `invalid DEM_KIND "other"`},
		},
		"missing rum clock": {
			fields: []envelopeField{{"DEM_KIND", "rum"}},
			want:   result{Err: "RUM history requires exactly one observation timestamp"},
		},
		"rum with start clock": {
			fields: []envelopeField{{"DEM_KIND", "rum"}, {"DEM_STARTED_US", "5"}},
			want:   result{Err: "RUM history requires exactly one observation timestamp"},
		},
		"rum with both clocks": {
			fields: []envelopeField{{"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "5"}, {"DEM_STARTED_US", "5"}},
			want:   result{Err: "RUM history requires exactly one observation timestamp"},
		},
		"synthetic with observation clock": {
			fields: []envelopeField{{"DEM_KIND", "synthetic"}, {"DEM_OBSERVED_US", "5"}},
			want:   result{Err: "synthetic history requires exactly one positive start timestamp"},
		},
		"duplicate clock": {
			fields: []envelopeField{{"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "5"}, {"DEM_OBSERVED_US", "5"}},
			want:   result{Err: "duplicate history field DEM_OBSERVED_US"},
		},
		"noncanonical clock": {
			fields: []envelopeField{{"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "05"}},
			want:   result{Err: "invalid DEM_OBSERVED_US: expected canonical nonnegative decimal"},
		},
		"reserved schema": {
			fields: []envelopeField{{"DEM_KIND", "rum"}, {"DEM_OBSERVED_US", "5"}, {"DEM_SCHEMA", "1"}},
			want:   result{Err: "history field DEM_SCHEMA is reserved for the journal envelope"},
		},
		"reserved rum bucket": {
			fields: []envelopeField{{"DEM_RUM_MINUTE", "0"}},
			want:   result{Err: "history field DEM_RUM_MINUTE is reserved for the journal envelope"},
		},
		"reserved synthetic bucket": {
			fields: []envelopeField{{"DEM_KIND", "synthetic"}, {"DEM_STARTED_US", "5"}, {"DEM_SYNTHETIC_MINUTE", "0"}},
			want:   result{Err: "history field DEM_SYNTHETIC_MINUTE is reserved for the journal envelope"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var input []journal.Field
			for _, field := range tc.fields {
				input = append(input, journal.StringField(field.name, field.value))
			}
			fields, kind, err := appendEnvelope(input)
			got := result{Kind: string(kind)}
			for _, field := range fields {
				got.Fields = append(got.Fields, envelopeField{field.Name, string(field.Value)})
			}
			if err != nil {
				got.Err = err.Error()
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
