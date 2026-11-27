// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadSMBServer(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		wantServer [len(smbServerCounters)]sample
		wantUsable bool
	}{
		"WinRM text and a malformed value": {
			rows: fixtureRows(map[string]any{"ServerSessions": "2", "ErrorsLogon": "malformed"}),
			wantServer: smbServerValues(map[string]sample{
				"ServerSessions": {value: 2, valid: true},
			}),
			wantUsable: true,
		},
		"no usable counter": {
			rows: fixtureRows(map[string]any{"ServerSessions": "malformed"}),
		},
		"more than one row": {
			rows: fixtureRows(smbServerRow(), smbServerRow()),
		},
		"no row": {
			rows: fixtureRows(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readSMBServer(tc.rows.rows))
			assert.Equal(t, tc.wantServer, o.smbServer)
		})
	}
}

func TestReadSMBShares(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		wantShares []smbShare
		wantUsable bool
	}{
		// A host whose Server service shares nothing.
		"no instance": {
			rows:       fixtureRows(),
			wantUsable: true,
		},
		"_Total only": {
			rows:       fixtureRows(map[string]any{"Name": "_Total", "TreeConnectCount": uint64(1)}),
			wantUsable: true,
		},
		"a share with a usable value": {
			rows: fixtureRows(
				map[string]any{"Name": `\\*\ADMIN$`, "TreeConnectCount": "malformed"},
				map[string]any{"Name": `\\*\C$`, "TreeConnectCount": "1", "SentBytesPersec": "malformed"},
				map[string]any{"TreeConnectCount": uint64(2)},
			),
			wantShares: []smbShare{
				{
					name: `\\*\C$`,
					values: smbShareValues(map[string]sample{
						"TreeConnectCount": {value: 1, valid: true},
					}),
				},
			},
			wantUsable: true,
		},
		"no share with a usable value": {
			rows: fixtureRows(
				map[string]any{"Name": `\\*\C$`, "TreeConnectCount": "malformed"},
				map[string]any{"Name": "_Total", "TreeConnectCount": uint64(1)},
			),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readSMBShares(tc.rows.rows))
			assert.Equal(t, tc.wantShares, o.smbShares)
		})
	}
}

// smbServerValues places samples, by property, where readSMBServer stores them.
func smbServerValues(values map[string]sample) [len(smbServerCounters)]sample {
	var samples [len(smbServerCounters)]sample
	for property, value := range values {
		samples[slices.IndexFunc(smbServerCounters[:], func(c rawCounter) bool { return c.property == property })] = value
	}
	return samples
}

// smbShareValues places samples, by property, where readSMBShares stores them.
func smbShareValues(values map[string]sample) [len(smbShareCounters)]sample {
	var samples [len(smbShareCounters)]sample
	for property, value := range values {
		samples[slices.IndexFunc(smbShareCounters[:], func(c rawCounter) bool { return c.property == property })] = value
	}
	return samples
}
