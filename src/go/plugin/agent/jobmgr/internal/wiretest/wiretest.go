// SPDX-License-Identifier: GPL-3.0-or-later

// Package wiretest provides shared test helpers for asserting plugin wire
// output as atomic protocol records. Multi-line FUNCTION_RESULT blocks are
// grouped into single records so assertions can never match across a record
// boundary; subsequence matching pins per-key ordering without constraining
// cross-key interleaving.
package wiretest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// RecordWant names one expected wire record and the substrings that identify
// it.
type RecordWant struct {
	Name     string
	Contains []string
}

// AtomicRecords splits wire output into atomic protocol records: one line per
// record, except FUNCTION_RESULT_BEGIN..FUNCTION_RESULT_END blocks, which
// form a single record.
func AtomicRecords(output string) []string {
	var records []string
	lines := strings.Split(output, "\n")
	for i := 0; i < len(lines); {
		line := lines[i]
		if line == "" {
			i++
			continue
		}
		if !strings.HasPrefix(line, "FUNCTION_RESULT_BEGIN ") {
			records = append(records, line)
			i++
			continue
		}

		var b strings.Builder
		b.WriteString(line)
		i++
		for i < len(lines) {
			b.WriteByte('\n')
			b.WriteString(lines[i])
			if lines[i] == "FUNCTION_RESULT_END" {
				i++
				break
			}
			i++
		}
		records = append(records, b.String())
	}
	return records
}

// findSubsequence reports whether wants appear in records in order (as a
// subsequence); when they do not, it returns the index of the first want
// that could not be found.
func findSubsequence(records []string, wants []RecordWant) (missing int, ok bool) {
	next := 0
	for wi, want := range wants {
		found := -1
		for i := next; i < len(records); i++ {
			matched := true
			for _, substr := range want.Contains {
				if !strings.Contains(records[i], substr) {
					matched = false
					break
				}
			}
			if matched {
				found = i
				break
			}
		}
		if found == -1 {
			return wi, false
		}
		next = found + 1
	}
	return 0, true
}

// RequireSubsequence asserts that wants appear in output's atomic records in
// order. Records not named by wants may appear anywhere between them.
func RequireSubsequence(t testing.TB, output string, wants []RecordWant) {
	t.Helper()

	records := AtomicRecords(output)
	missing, ok := findSubsequence(records, wants)
	if !ok {
		require.Failf(t, "wire record subsequence mismatch",
			"record %q (index %d) not found in order in records:\n%s",
			wants[missing].Name, missing, strings.Join(records, "\n---\n"))
	}
}
