// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadTerminalServices(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		wantValues [len(terminalServicesCounters)]sample
		wantUsable bool
	}{
		"DCOM types": {
			rows: fixtureRows(terminalServicesRow()),
			wantValues: [len(terminalServicesCounters)]sample{
				{value: 4, valid: true},
				{value: 7, valid: true},
			},
			wantUsable: true,
		},
		// No connected user is a reported value, not a gap.
		"WinRM text, a valid zero and a malformed value": {
			rows: fixtureRows(map[string]any{
				"ActiveSessions":   "0",
				"InactiveSessions": "malformed",
			}),
			wantValues: [len(terminalServicesCounters)]sample{
				{value: 0, valid: true},
				{},
			},
			wantUsable: true,
		},
		"no usable counter": {
			rows: fixtureRows(map[string]any{"ActiveSessions": "malformed"}),
		},
		"more than one row": {
			rows: fixtureRows(terminalServicesRow(), terminalServicesRow()),
		},
		"no row": {
			rows: fixtureRows(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readTerminalServices(tc.rows.rows))
			assert.Equal(t, tc.wantValues, o.terminalServices)
		})
	}
}
