// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/journaltest"
)

func requireJournalctl(t testing.TB) {
	t.Helper()
	journaltest.RequireJournalctl(t)
}

func runJournalctlAllowEmpty(t testing.TB, dir, match string) string {
	t.Helper()
	return journaltest.RunJournalctlAllowEmpty(t, dir, match)
}
