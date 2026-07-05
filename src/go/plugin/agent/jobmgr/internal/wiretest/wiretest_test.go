// SPDX-License-Identifier: GPL-3.0-or-later

package wiretest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleOutput = `CONFIG a create

FUNCTION_RESULT_BEGIN uid-1 200 application/json
{"status":200}
FUNCTION_RESULT_END

CONFIG a status running

CONFIG b create

FUNCTION_RESULT_BEGIN uid-2 200 application/json
{"status":200}
FUNCTION_RESULT_END
`

func TestAtomicRecords(t *testing.T) {
	tests := map[string]struct {
		output      string
		wantRecords int
	}{
		"function result blocks group into one record": {
			output:      sampleOutput,
			wantRecords: 5,
		},
		"empty output yields no records": {
			output:      "\n\n",
			wantRecords: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			records := AtomicRecords(tc.output)
			assert.Len(t, records, tc.wantRecords)
			for _, r := range records {
				assert.NotContains(t, r[1:], "FUNCTION_RESULT_BEGIN",
					"a record must never contain a second result block")
			}
		})
	}
}

// The negative cases prove the harness REJECTS same-key reorderings - a
// subsequence assertion that cannot fail would let per-key ordering
// regressions through silently.
func TestFindSubsequence(t *testing.T) {
	tests := map[string]struct {
		wants       []RecordWant
		wantOK      bool
		wantMissing int
	}{
		"in-order wants match as a subsequence": {
			wants: []RecordWant{
				{Name: "a create", Contains: []string{"CONFIG a create"}},
				{Name: "uid-1", Contains: []string{"FUNCTION_RESULT_BEGIN uid-1"}},
				{Name: "a running", Contains: []string{"CONFIG a status running"}},
			},
			wantOK: true,
		},
		"unrelated records may interleave": {
			wants: []RecordWant{
				{Name: "a create", Contains: []string{"CONFIG a create"}},
				{Name: "b create", Contains: []string{"CONFIG b create"}},
			},
			wantOK: true,
		},
		"reordered wants are rejected": {
			wants: []RecordWant{
				{Name: "a running", Contains: []string{"CONFIG a status running"}},
				{Name: "uid-1", Contains: []string{"FUNCTION_RESULT_BEGIN uid-1"}},
			},
			wantOK:      false,
			wantMissing: 1,
		},
		"missing record is rejected": {
			wants: []RecordWant{
				{Name: "ghost", Contains: []string{"CONFIG ghost"}},
			},
			wantOK:      false,
			wantMissing: 0,
		},
		"multi-substring want must match within ONE record": {
			wants: []RecordWant{
				{Name: "cross-record", Contains: []string{"CONFIG a create", "CONFIG b create"}},
			},
			wantOK:      false,
			wantMissing: 0,
		},
	}

	records := AtomicRecords(sampleOutput)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			missing, ok := findSubsequence(records, tc.wants)
			require.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				assert.Equal(t, tc.wantMissing, missing)
			}
		})
	}
}
