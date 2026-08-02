// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"os/exec"
	"strings"
	"testing"
)

func requireJournalctl(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Skip("journalctl not found")
	}
}

func runJournalctlAllowEmpty(t testing.TB, dir, match string) string {
	t.Helper()
	cmd := exec.Command("journalctl", "--directory="+dir, match, "-o", "json", "--no-pager")
	out, err := cmd.CombinedOutput()
	if err != nil && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("journalctl failed: %v\n%s", err, string(out))
	}
	return string(out)
}
