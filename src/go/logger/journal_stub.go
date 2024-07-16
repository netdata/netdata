// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package logger

func isStderrConnectedToJournal() bool {
	return false
}
