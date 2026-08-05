// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
)

var isTerm = terminal.IsTerminal()

var isJournal = isStderrConnectedToJournal()

var pluginAttr = slog.String("plugin", executable.Name)

func New() *Logger {
	return newLogger(os.Stderr, isTerm, 4)
}

func NewWithWriter(w io.Writer) *Logger {
	if w == nil {
		w = os.Stderr
	}
	return newLogger(w, false, 4)
}

func newLogger(w io.Writer, isTerminal bool, depth int) *Logger {
	if isTerminal {
		// skip 2 slog pkg calls, 2 this pkg calls
		return &Logger{sl: slog.New(withTerminalCallDepth(depth, newTerminalHandler(w))), rl: newRateLimiter()}
	}
	return &Logger{sl: slog.New(newTextHandler(w)).With(pluginAttr), rl: newRateLimiter()}
}

type Logger struct {
	muted            atomic.Bool
	sl               *slog.Logger
	rl               *rateLimiter
	messageSanitizer func(string) string
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
	args = sanitizeLogAttributes(l.messageSanitizer, args)

	ll := &Logger{sl: l.sl.With(args...), rl: l.rl, messageSanitizer: l.messageSanitizer}
	ll.muted.Store(l.muted.Load())

	return ll
}

func sanitizeLogAttributes(sanitize func(string) string, attributes []any) []any {
	if sanitize == nil || len(attributes) == 0 {
		return attributes
	}
	sanitized := make([]any, 0, len(attributes))
	for index := 0; index < len(attributes); index++ {
		switch attribute := attributes[index].(type) {
		case slog.Attr:
			sanitized = append(sanitized, slog.String(
				"redacted_attribute",
				sanitizeLogMessage(sanitize, fmt.Sprint(attribute.Value.Any())),
			))
		case string:
			sanitized = append(sanitized, "redacted_attribute")
			if index+1 < len(attributes) {
				index++
				sanitized = append(
					sanitized,
					sanitizeLogMessage(sanitize, fmt.Sprint(attributes[index])),
				)
			}
		default:
			sanitized = append(sanitized, sanitizeLogMessage(sanitize, fmt.Sprint(attribute)))
		}
	}
	return sanitized
}

// WithMessageSanitizer returns a derived logger that sanitizes messages and
// attributes added after this call. Existing attributes remain unchanged.
func (l *Logger) WithMessageSanitizer(sanitize func(string) string) *Logger {
	if l.isNil() {
		l = New()
	}
	ll := &Logger{sl: l.sl, rl: l.rl, messageSanitizer: sanitize}
	ll.muted.Store(l.muted.Load())
	return ll
}

func (l *Logger) log(level slog.Level, msg string) {
	if l.isNil() {
		nilLogger.sl.Log(context.Background(), level, msg)
		return
	}

	if !l.muted.Load() {
		msg = sanitizeLogMessage(l.messageSanitizer, msg)
		l.sl.Log(context.Background(), level, msg)
	}
}

func sanitizeLogMessage(sanitize func(string) string, message string) (sanitized string) {
	if sanitize == nil {
		return message
	}
	sanitized = "log message redacted"
	defer func() {
		_ = recover()
	}()
	if result := sanitize(message); result != "" {
		sanitized = result
	}
	return sanitized
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
