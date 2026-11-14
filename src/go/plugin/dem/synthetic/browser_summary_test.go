// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserSummaryCoveragePresentation(t *testing.T) {
	var absent *BrowserSummary
	assert.Equal(t, "not_recorded", absent.ReportingStatus())
	assert.Empty(t, absent.Summary())
	value := &BrowserSummary{Availability: "supported", Coverage: "fixture_context", ReporterComplete: true, Run: &BrowserAggregate{Tests: BrowserTestCounts{NoSummary: 2}}}
	assert.Contains(t, value.Summary(), "no observed fixture contexts")
	assert.Contains(t, value.Summary(), "no_summary=2")
	assert.NotContains(t, value.Summary(), "uncaught=0")
	value.ReporterComplete = false
	value.Run.Tests = BrowserTestCounts{Observed: 1, Incomplete: 2, Invalid: 3, NoSummary: 4}
	value.Run.Counts.ConsoleError = 5
	value.Run.Omissions = []string{"pending"}
	assert.Contains(t, value.Summary(), "partial")
	assert.Contains(t, value.Summary(), "observed=1, incomplete=2, invalid=3, no_summary=4")
	assert.Contains(t, value.Summary(), "observed errors:")
	assert.Contains(t, value.Summary(), "console_error=5")
	assert.Contains(t, value.Summary(), "pending")
	value.Availability = "invalid"
	assert.Equal(t, "invalid", value.ReportingStatus())
	assert.Contains(t, value.Summary(), "console_error=5")
}

// TestBrowserStoredJSONRoundTrip covers the stored shapes; wire strictness is
// the runner's (TestBrowserWireRejectsNullAndAmbiguousMembers).
func TestBrowserStoredJSONRoundTrip(t *testing.T) {
	var a BrowserAggregate
	require.NoError(t, json.Unmarshal([]byte(`{"tests":{"observed":1},"counts":{}}`), &a))
	assert.Equal(t, BrowserAggregate{Tests: BrowserTestCounts{Observed: 1}}, a)
	for _, raw := range []string{`{"value":null}`, `{"value":false}`, `{"value":1.25}`, `{"value":"text"}`, `{"omitted":"unrepresentable"}`} {
		var arg BrowserArgument
		require.NoError(t, json.Unmarshal([]byte(raw), &arg))
		out, err := json.Marshal(arg)
		require.NoError(t, err)
		assert.JSONEq(t, raw, string(out))
	}
}
