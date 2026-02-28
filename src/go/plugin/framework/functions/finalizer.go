// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "sync"

type finalizeHookFn func(uid, source string, emit func()) bool

var (
	finalizeHookMux sync.RWMutex
	finalizeHook    finalizeHookFn
)

func setFinalizeHook(h finalizeHookFn) (restore func()) {
	finalizeHookMux.Lock()
	prev := finalizeHook
	finalizeHook = h
	finalizeHookMux.Unlock()

	return func() {
		finalizeHookMux.Lock()
		finalizeHook = prev
		finalizeHookMux.Unlock()
	}
}

// FinalizeTerminal routes terminal response emission through an optional finalize hook.
// If no hook is set, emit is invoked directly.
func FinalizeTerminal(uid, source string, emit func()) bool {
	if emit == nil {
		return false
	}

	finalizeHookMux.RLock()
	h := finalizeHook
	finalizeHookMux.RUnlock()

	if h == nil {
		emit()
		return true
	}

	return h(uid, source, emit)
}
