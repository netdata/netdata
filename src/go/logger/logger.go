// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/executable"

	"github.com/mattn/go-isatty"
)

var isTerm = isatty.IsTerminal(os.Stderr.Fd())

var isJournal = isStderrConnectedToJournal()

var pluginAttr = slog.String("plugin", executable.Name)

func New() *Logger {
	if isTerm {
		// skip 2 slog pkg calls, 2 this pkg calls
		return &Logger{sl: slog.New(withCallDepth(4, newTerminalHandler())), rl: newRateLimiter()}
	}
	return &Logger{sl: slog.New(newTextHandler()).With(pluginAttr), rl: newRateLimiter()}
}

type Logger struct {
	muted atomic.Bool
	sl    *slog.Logger
	rl    *rateLimiter
}

func (l *Logger) Error(a ...any) {
	if !l.canLog(slog.LevelError) {
		return
	}
	l.log(slog.LevelError, fmt.Sprint(a...))
}

func (l *Logger) Warning(a ...any) {
	if !l.canLog(slog.LevelWarn) {
		return
	}
	l.log(slog.LevelWarn, fmt.Sprint(a...))
}

func (l *Logger) Notice(a ...any) {
	if !l.canLog(levelNotice) {
		return
	}
	l.log(levelNotice, fmt.Sprint(a...))
}

func (l *Logger) Info(a ...any) {
	if !l.canLog(slog.LevelInfo) {
		return
	}
	l.log(slog.LevelInfo, fmt.Sprint(a...))
}

func (l *Logger) Debug(a ...any) {
	if !l.canLog(slog.LevelDebug) {
		return
	}
	l.log(slog.LevelDebug, fmt.Sprint(a...))
}

func (l *Logger) Errorf(format string, a ...any) {
	if !l.canLog(slog.LevelError) {
		return
	}
	l.log(slog.LevelError, fmt.Sprintf(format, a...))
}

func (l *Logger) Warningf(format string, a ...any) {
	if !l.canLog(slog.LevelWarn) {
		return
	}
	l.log(slog.LevelWarn, fmt.Sprintf(format, a...))
}

func (l *Logger) Noticef(format string, a ...any) {
	if !l.canLog(levelNotice) {
		return
	}
	l.log(levelNotice, fmt.Sprintf(format, a...))
}

func (l *Logger) Infof(format string, a ...any) {
	if !l.canLog(slog.LevelInfo) {
		return
	}
	l.log(slog.LevelInfo, fmt.Sprintf(format, a...))
}

func (l *Logger) Debugf(format string, a ...any) {
	if !l.canLog(slog.LevelDebug) {
		return
	}
	l.log(slog.LevelDebug, fmt.Sprintf(format, a...))
}

func (l *Logger) Mute()   { l.mute(true) }
func (l *Logger) Unmute() { l.mute(false) }

func (l *Logger) With(args ...any) *Logger {
	if l.isNil() {
		ll := New()
		return &Logger{sl: ll.sl.With(args...), rl: ll.rl}
	}

	ll := &Logger{sl: l.sl.With(args...), rl: l.rl}
	ll.muted.Store(l.muted.Load())

	return ll
}

func (l *Logger) log(level slog.Level, msg string) {
	if l.isNil() {
		nilLogger.sl.Log(context.Background(), level, msg)
		return
	}

	if !l.muted.Load() {
		l.sl.Log(context.Background(), level, msg)
	}
}

func (l *Logger) canLog(level slog.Level) bool {
	if !Level.Enabled(level) {
		return false
	}
	if l.isNil() {
		return true
	}
	return !l.muted.Load()
}

func (l *Logger) mute(v bool) {
	if l.isNil() || isTerm && Level.Enabled(slog.LevelDebug) {
		return
	}
	l.muted.Store(v)
}

func (l *Logger) isNil() bool { return l == nil || l.sl == nil }

var nilLogger = New()
