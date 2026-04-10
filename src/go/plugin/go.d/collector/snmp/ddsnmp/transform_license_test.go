// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runLicenseTransform compiles a transform body and applies it to the given
// metric. It mirrors the minimal "execute the template against {Metric: m}"
// contract used by ddsnmpcollector.applyTransform, without crossing package
// boundaries just for the test.
func runLicenseTransform(t *testing.T, body string, m *Metric) {
	t.Helper()
	require.NoError(t, executeLicenseTransform(body, m))
}

func executeLicenseTransform(body string, m *Metric) error {
	tmpl, err := compileTransform(body)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	return tmpl.Execute(&buf, struct{ Metric *Metric }{Metric: m})
}

func TestSetTagTransform_StampsValueKindOnTagsMap(t *testing.T) {
	m := &Metric{Value: 42, Tags: map[string]string{}}
	runLicenseTransform(t, `{{- setTag .Metric "_license_value_kind" "expiry_timestamp" -}}`, m)

	assert.Equal(t, "expiry_timestamp", m.Tags["_license_value_kind"])
	assert.EqualValues(t, 42, m.Value)
}

func TestSetTagTransform_AllocatesTagsWhenNil(t *testing.T) {
	m := &Metric{Value: 1}
	runLicenseTransform(t, `{{- setTag .Metric "_license_value_kind" "state_severity" -}}`, m)

	require.NotNil(t, m.Tags)
	assert.Equal(t, "state_severity", m.Tags["_license_value_kind"])
}

func TestLicenseDateFromTagTransform_ParsesISODate(t *testing.T) {
	m := &Metric{
		Value: 0,
		Tags:  map[string]string{"_license_expiry_text": "2026-12-31"},
	}
	runLicenseTransform(t, `{{- licenseDateFromTag .Metric "_license_expiry_text" "expiry_timestamp" -}}`, m)

	assert.Equal(t, "expiry_timestamp", m.Tags["_license_value_kind"])
	// 2026-12-31 00:00:00 UTC
	assert.EqualValues(t, 1798675200, m.Value)
}

func TestLicenseDateFromTagTransform_ParsesEpochSeconds(t *testing.T) {
	m := &Metric{Value: 0, Tags: map[string]string{"x": "1798675200"}}
	runLicenseTransform(t, `{{- licenseDateFromTag .Metric "x" "expiry_timestamp" -}}`, m)
	assert.EqualValues(t, 1798675200, m.Value)
}

func TestLicenseDateFromTagTransform_ParsesEpochMillis(t *testing.T) {
	m := &Metric{Value: 0, Tags: map[string]string{"x": "1798675200000"}}
	runLicenseTransform(t, `{{- licenseDateFromTag .Metric "x" "expiry_timestamp" -}}`, m)
	assert.EqualValues(t, 1798675200, m.Value)
}

func TestLicenseDateFromTagTransform_ParsesTwelveDigitEpochMillis(t *testing.T) {
	m := &Metric{Value: 0, Tags: map[string]string{"x": "946684800000"}}
	runLicenseTransform(t, `{{- licenseDateFromTag .Metric "x" "expiry_timestamp" -}}`, m)
	assert.EqualValues(t, 946684800, m.Value)
}

func TestLicenseDateFromTagTransform_ParsesCheckpointShortDate(t *testing.T) {
	// Checkpoint sends licensingExpirationDate as "2Jan2030", "1Jan2030", etc.
	m := &Metric{Value: 0, Tags: map[string]string{"x": "1Jan2030"}}
	runLicenseTransform(t, `{{- licenseDateFromTag .Metric "x" "expiry_timestamp" -}}`, m)
	assert.NotZero(t, m.Value)
}

func TestLicenseDateFromTagTransform_RejectsAmbiguousSlashDate(t *testing.T) {
	m := &Metric{Value: 999, Tags: map[string]string{"x": "01/02/2024"}}
	runLicenseTransform(t, `{{- licenseDateFromTag .Metric "x" "expiry_timestamp" -}}`, m)

	assert.Empty(t, m.Tags["_license_value_kind"])
	assert.EqualValues(t, 999, m.Value)
}

func TestLicenseDateFromTagTransform_RejectsSentinels(t *testing.T) {
	cases := []string{"0", "never", "perpetual", "n/a", "4294967295", ""}
	for _, raw := range cases {
		m := &Metric{Value: 999, Tags: map[string]string{"x": raw}}
		runLicenseTransform(t, `{{- licenseDateFromTag .Metric "x" "expiry_timestamp" -}}`, m)
		// Untouched: no value_kind stamp, original value preserved.
		assert.Empty(t, m.Tags["_license_value_kind"], "raw=%q", raw)
		assert.EqualValues(t, 999, m.Value, "raw=%q", raw)
	}
}

func TestLicenseDateFromTagTransform_RejectsUnsupportedKind(t *testing.T) {
	for _, kind := range []string{"usage", "expiry_remaining", "not_a_kind"} {
		m := &Metric{Value: 999, Tags: map[string]string{"x": "2026-12-31"}}
		err := executeLicenseTransform(`{{- licenseDateFromTag .Metric "x" "`+kind+`" -}}`, m)

		require.Error(t, err, "kind=%q", kind)
		assert.Contains(t, err.Error(), `licenseDateFromTag: unsupported value kind`, "kind=%q", kind)
		assert.Empty(t, m.Tags["_license_value_kind"], "kind=%q", kind)
		assert.EqualValues(t, 999, m.Value, "kind=%q", kind)
	}
}

func TestIsTextDateNoValue(t *testing.T) {
	noValues := []string{"", "0", "-1", "never", "perpetual", "permanent", "n/a", "na", "none", "unlimited", "4294967295"}
	for _, raw := range noValues {
		assert.True(t, IsTextDateNoValue(raw), "raw=%q", raw)
	}

	values := []string{"1", "1798675200", "2026-12-31", "not-a-date"}
	for _, raw := range values {
		assert.False(t, IsTextDateNoValue(raw), "raw=%q", raw)
	}
}

func TestLicenseDateFromTagTransform_NoTagsMapIsNoop(t *testing.T) {
	m := &Metric{Value: 7}
	runLicenseTransform(t, `{{- licenseDateFromTag .Metric "x" "expiry_timestamp" -}}`, m)
	assert.EqualValues(t, 7, m.Value)
}
