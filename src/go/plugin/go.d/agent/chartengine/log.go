// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

func (e *Engine) logDebugf(format string, args ...any) {
	if e == nil || e.state.log == nil {
		return
	}
	e.state.log.Debugf(format, args...)
}

func (e *Engine) logInfof(format string, args ...any) {
	if e == nil || e.state.log == nil {
		return
	}
	e.state.log.Infof(format, args...)
}

func (e *Engine) logWarningf(format string, args ...any) {
	if e == nil || e.state.log == nil {
		return
	}
	e.state.log.Warningf(format, args...)
}
