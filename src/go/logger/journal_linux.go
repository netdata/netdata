// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package logger

import (
	"github.com/coreos/go-systemd/v22/journal"
)

func isStderrConnectedToJournal() bool {
	ok, _ := journal.StderrIsJournalStream()
	return ok
}
