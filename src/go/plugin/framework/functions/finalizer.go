// SPDX-License-Identifier: GPL-3.0-or-later

package functions

// TerminalFinalizer routes terminal response emission for a transaction UID.
type TerminalFinalizer func(uid, source string, emit func()) bool

// DirectTerminalFinalizer emits a terminal response without manager-level
// deduplication.
func DirectTerminalFinalizer(_ string, _ string, emit func()) bool {
	if emit == nil {
		return false
	}
	emit()
	return true
}
