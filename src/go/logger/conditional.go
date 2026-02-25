// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"fmt"
	"log/slog"
)

type ConditionalLogger struct {
	l         *Logger
	cond      bool
	allowElse bool
}

type ElseBranch struct {
	l    *Logger
	cond bool
}

func (l *Logger) When(cond bool) ConditionalLogger {
	return ConditionalLogger{l: l, cond: cond, allowElse: true}
}

func (b ElseBranch) Else() ConditionalLogger {
	return ConditionalLogger{l: b.l, cond: b.cond}
}

func (c ConditionalLogger) Error(a ...any) ElseBranch {
	return c.logArgs(slog.LevelError, a...)
}

func (c ConditionalLogger) Warning(a ...any) ElseBranch {
	return c.logArgs(slog.LevelWarn, a...)
}

func (c ConditionalLogger) Notice(a ...any) ElseBranch {
	return c.logArgs(levelNotice, a...)
}

func (c ConditionalLogger) Info(a ...any) ElseBranch {
	return c.logArgs(slog.LevelInfo, a...)
}

func (c ConditionalLogger) Debug(a ...any) ElseBranch {
	return c.logArgs(slog.LevelDebug, a...)
}

func (c ConditionalLogger) Errorf(format string, a ...any) ElseBranch {
	return c.logf(slog.LevelError, format, a...)
}

func (c ConditionalLogger) Warningf(format string, a ...any) ElseBranch {
	return c.logf(slog.LevelWarn, format, a...)
}

func (c ConditionalLogger) Noticef(format string, a ...any) ElseBranch {
	return c.logf(levelNotice, format, a...)
}

func (c ConditionalLogger) Infof(format string, a ...any) ElseBranch {
	return c.logf(slog.LevelInfo, format, a...)
}

func (c ConditionalLogger) Debugf(format string, a ...any) ElseBranch {
	return c.logf(slog.LevelDebug, format, a...)
}

func (c ConditionalLogger) logArgs(level slog.Level, a ...any) ElseBranch {
	if c.cond && c.l.canLog(level) {
		c.l.log(level, fmt.Sprint(a...))
	}
	return c.next()
}

func (c ConditionalLogger) logf(level slog.Level, format string, a ...any) ElseBranch {
	if c.cond && c.l.canLog(level) {
		c.l.log(level, fmt.Sprintf(format, a...))
	}
	return c.next()
}

func (c ConditionalLogger) next() ElseBranch {
	cond := false
	if c.allowElse {
		cond = !c.cond
	}
	return ElseBranch{l: c.l, cond: cond}
}
