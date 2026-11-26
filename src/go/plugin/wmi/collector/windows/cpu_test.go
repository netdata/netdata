// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The oracle is the native chart's percent-of-total-delta algorithm
// (RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL in src/database/rrdset-collection.c):
// each mode's increase over the increase of all five modes together.
func TestModeShares(t *testing.T) {
	for name, tc := range map[string]struct {
		prev, cur cpuTimes
		want      cpuShares
		wantOK    bool
	}{
		"share of the summed increase": {
			prev:   cpuTimes{10, 100, 50, 5, 1000},
			cur:    cpuTimes{20, 300, 150, 15, 1680},
			want:   cpuShares{1, 20, 10, 1, 68},
			wantOK: true,
		},
		// Privileged time includes interrupt and DPC time, which the native
		// chart also shows on their own; the total counts them twice.
		"interrupt and DPC time within privileged time": {
			prev:   cpuTimes{0, 0, 0, 0, 0},
			cur:    cpuTimes{100, 0, 300, 100, 500},
			want:   cpuShares{10, 0, 30, 10, 50},
			wantOK: true,
		},
		"fully idle": {
			prev:   cpuTimes{1, 2, 3, 4, 5},
			cur:    cpuTimes{1, 2, 3, 4, 805},
			want:   cpuShares{0, 0, 0, 0, 100},
			wantOK: true,
		},
		"no time accumulated": {
			prev: cpuTimes{1, 2, 3, 4, 5},
			cur:  cpuTimes{1, 2, 3, 4, 5},
		},
		"a counter went back": {
			prev: cpuTimes{1, 2, 3, 4, 500},
			cur:  cpuTimes{9, 9, 9, 9, 400},
		},
	} {
		t.Run(name, func(t *testing.T) {
			shares, ok := modeShares(tc.prev, tc.cur)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, shares)
		})
	}
}

func TestReadCPU(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		want       cpuSample
		wantUsable bool
	}{
		// The native plugin labels a core cpuN by its global processor number.
		"cores numbered in node and index order": {
			rows: fixtureRows(
				processorRow[uint64]("_Total", 1, 2, 3, 4, 5),
				processorRow[uint64]("1,_Total", 9, 9, 9, 9, 9),
				processorRow[uint64]("1,0", 3, 3, 3, 3, 3),
				processorRow[uint64]("0,10", 2, 2, 2, 2, 2),
				processorRow[uint64]("0,9", 1, 1, 1, 1, 1),
			),
			want: cpuSample{
				total:    cpuTimes{1, 2, 3, 4, 5},
				hasTotal: true,
				cores: map[string]cpuTimes{
					"cpu0": {1, 1, 1, 1, 1},
					"cpu1": {2, 2, 2, 2, 2},
					"cpu2": {3, 3, 3, 3, 3},
				},
			},
			wantUsable: true,
		},
		"rows without every counter or with unknown names are skipped": {
			rows: fixtureRows(
				map[string]any{"Name": "_Total", "PercentUserTime": uint64(1)},
				processorRow("0,0", "1", "2", "3", "4", "-5"),
				processorRow[uint64]("Processor 7", 1, 1, 1, 1, 1),
				processorRow[uint64]("0,1", 1, 2, 3, 4, 5),
			),
			want: cpuSample{
				cores: map[string]cpuTimes{
					"cpu0": {1, 2, 3, 4, 5},
				},
			},
			wantUsable: true,
		},
		"no usable row": {
			rows: fixtureRows(map[string]any{"Name": "_Total"}),
			want: cpuSample{
				cores: map[string]cpuTimes{},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readCPU(tc.rows.rows))
			assert.Equal(t, tc.want, o.cpuTimes)
		})
	}
}

func TestCPUUtilizationBetween(t *testing.T) {
	prev := cpuSample{
		total:    cpuTimes{0, 0, 0, 0, 0},
		hasTotal: true,
		cores: map[string]cpuTimes{
			"cpu0": {0, 0, 0, 0, 0},
			"cpu1": {0, 0, 0, 0, 0},
		},
	}
	cur := cpuSample{
		total:    cpuTimes{0, 25, 25, 0, 50},
		hasTotal: true,
		cores: map[string]cpuTimes{
			"cpu0": {0, 50, 0, 0, 50},
			"cpu1": {0, 0, 0, 0, 0},
			"cpu2": {0, 100, 0, 0, 0},
		},
	}
	assert.Equal(t, cpuUtilization{
		total:    cpuShares{0, 25, 25, 0, 50},
		hasTotal: true,
		cores: map[string]cpuShares{
			"cpu0": {0, 50, 0, 0, 50},
		},
	}, cpuUtilizationBetween(prev, cur), "cpu1 accumulated no time and cpu2 has no previous sample")
	assert.Equal(t, cpuUtilization{}, cpuUtilizationBetween(cpuSample{}, cur), "no previous sample")
}
