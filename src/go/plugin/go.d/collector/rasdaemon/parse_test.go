// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package rasdaemon

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err, "reading fixture %s", name)
	return data
}

// TestParseSummary_KnownBad is the load-bearing test. The host this collector was developed on has
// never recorded a single RAS error, so a parser exercised only against healthy output could not be
// shown to fail. The fixture is real `ras-mc-ctl --summary` output captured from a synthetic
// database seeded with known-bad rows (see make-fixtures.sh).
func TestParseSummary_KnownBad(t *testing.T) {
	sm, err := parseSummary(loadFixture(t, "summary-with-errors.txt"))
	require.NoError(t, err)

	// One physical DIMM is reported once per csrow, so DDR4_A1 spans two input lines
	// (2 + 1). Failing to sum them is the single most likely parser bug, and it would
	// under-report a failing stick by half.
	assert.ElementsMatch(t, []dimmEvents{
		{dimm: "DDR4_A1", errType: "Corrected", count: 3},
		{dimm: "DDR4_B1", errType: "Corrected", count: 1},
		{dimm: "DDR4_G1", errType: "Uncorrected", count: 1},
	}, sm.memory)

	// AER: count comes FIRST on the line and the trailing free-form message is discarded.
	assert.ElementsMatch(t, []typedEvents{
		{errType: "Corrected", count: 2},
		{errType: "Fatal", count: 1},
	}, sm.aer)

	// MCE: count first, no trailing message.
	assert.ElementsMatch(t, []typedEvents{
		{errType: "Corrected error", count: 1},
		{errType: "Uncorrected error", count: 1},
	}, sm.mce)

	// Memory failure: count comes LAST. Getting this backwards would report the action
	// result as a count.
	assert.ElementsMatch(t, []typedEvents{
		{errType: "Recovered", count: 1},
	}, sm.memoryFailure)

	assert.Equal(t, map[string]int64{
		classDevlink: 1,
		classDisk:    1,
		classSignal:  1,
	}, sm.classes)
}

// TestParseSummary_KnownGood pins the healthy case: every section present, all empty, no error.
func TestParseSummary_KnownGood(t *testing.T) {
	sm, err := parseSummary(loadFixture(t, "summary-no-errors.txt"))
	require.NoError(t, err)

	assert.Empty(t, sm.memory)
	assert.Empty(t, sm.aer)
	assert.Empty(t, sm.mce)
	assert.Empty(t, sm.memoryFailure)
	assert.Empty(t, sm.classes)
	assert.True(t, sm.isEmpty())
}

// TestParseSummary_Errors covers inputs that MUST be reported as failures rather than as
// "zero errors". A healthy-looking zero here would mask a broken RAS pipeline.
func TestParseSummary_Errors(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"empty input (ras-mc-ctl died on a missing table)": {
			input: "",
		},
		"whitespace only": {
			input: "\n\n   \n\t\n",
		},
		"output with no recognizable section": {
			input: "ras-mc-ctl: some unexpected diagnostic\nanother line\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sm, err := parseSummary([]byte(test.input))
			assert.Error(t, err)
			assert.Nil(t, sm)
		})
	}
}

// TestParseSummary_MissingTablesFixture asserts the real degenerate capture is rejected.
func TestParseSummary_MissingTablesFixture(t *testing.T) {
	sm, err := parseSummary(loadFixture(t, "summary-missing-tables.txt"))
	assert.Error(t, err)
	assert.Nil(t, sm)
}

// TestParseSummary_SectionAmbiguity is a regression guard for the core hazard in this format:
// the per-line grammars collide across sections. Each line below is valid in more than one
// section and only the surrounding header disambiguates it.
func TestParseSummary_SectionAmbiguity(t *testing.T) {
	tests := map[string]struct {
		input       string
		wantAER     []typedEvents
		wantMemFail []typedEvents
		wantClasses map[string]int64
	}{
		"'N X errors: msg' under AER is count-first": {
			input:       "PCIe AER events summary:\n\t7 Corrected errors: Receiver Error\n",
			wantAER:     []typedEvents{{errType: "Corrected", count: 7}},
			wantClasses: map[string]int64{},
		},
		"'X errors: N' under memory failure is count-last": {
			input:       "Memory failure events summary:\n\tDelayed errors: 9\n",
			wantMemFail: []typedEvents{{errType: "Delayed", count: 9}},
			wantClasses: map[string]int64{},
		},
		"'N errors: M' under SIGNAL takes the trailing count": {
			// sigcode 2 with 14 occurrences must yield 14, not 2.
			input:       "SIGNAL events summary:\n\t2 errors: 14\n",
			wantClasses: map[string]int64{classSignal: 14},
		},
		"'dev has N errors' under disk": {
			input:       "Disk errors summary:\n\tsdd has 5 errors\n",
			wantClasses: map[string]int64{classDisk: 5},
		},
		"CXL header with the upstream double-space typo is still recognized": {
			input:       "CXL  generic events summary:\n\tmem0 errors: 4\n",
			wantClasses: map[string]int64{classCXL: 4},
		},
		"detail lines outside any section are ignored": {
			input:       "No Memory errors.\n\t3 Corrected errors: stray\n",
			wantClasses: map[string]int64{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sm, err := parseSummary([]byte(test.input))
			require.NoError(t, err)
			assert.ElementsMatch(t, test.wantAER, sm.aer, "aer")
			assert.ElementsMatch(t, test.wantMemFail, sm.memoryFailure, "memoryFailure")
			assert.Equal(t, test.wantClasses, sm.classes, "classes")
		})
	}
}

// TestParseSummary_MultipleCXLSectionsAggregate verifies the seven CXL section variants all fold
// into one bounded class total rather than creating seven near-empty metrics.
func TestParseSummary_MultipleCXLSectionsAggregate(t *testing.T) {
	input := "CXL AER uncorrectable events summary:\n\tmem0 errors: 1\n" +
		"CXL DRAM events summary:\n\tmem0 errors: 2\n" +
		"CXL poison events summary:\n\tmem1 errors: 3\n"

	sm, err := parseSummary([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{classCXL: 6}, sm.classes)
}
