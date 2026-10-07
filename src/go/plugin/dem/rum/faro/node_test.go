// SPDX-License-Identifier: GPL-3.0-or-later
package faro

import (
	"os/exec"
	"testing"
)

func requireNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required to execute the shipped SDK browser fixtures")
	}
}
