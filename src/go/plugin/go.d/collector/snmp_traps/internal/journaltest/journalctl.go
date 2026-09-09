// SPDX-License-Identifier: GPL-3.0-or-later

package journaltest

import (
	"os/exec"
	"strings"
	"testing"
)

func RequireJournalctl(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Skip("journalctl not found")
	}
}

func RunJournalctl(t testing.TB, dir, match string) string {
	t.Helper()
	out := RunJournalctlAllowEmpty(t, dir, match)
	if strings.TrimSpace(out) == "" {
		t.Fatalf("journalctl returned empty output for %s in %s", match, dir)
	}
	return out
}

func RunJournalctlAllowEmpty(t testing.TB, dir, match string) string {
	t.Helper()
	cmd := exec.Command("journalctl", "--directory="+dir, match, "-o", "json", "--no-pager")
	out, err := cmd.CombinedOutput()
	if err != nil && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("journalctl failed: %v\n%s", err, string(out))
	}
	return string(out)
}
