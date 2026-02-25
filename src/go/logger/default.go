// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"log/slog"
	"os"
	"time"

	"github.com/mattn/go-isatty"
)

func newDefaultLogger() *Logger {
	if isatty.IsTerminal(os.Stderr.Fd()) {
		// skip 2 slog pkg calls, 3 this pkg calls
		return &Logger{sl: slog.New(withCallDepth(5, newTerminalHandler())), rl: newRateLimiter()}
	}
	return &Logger{sl: slog.New(newTextHandler()).With(pluginAttr), rl: newRateLimiter()}
}

var defaultLogger = newDefaultLogger()

func Error(a ...any)                   { defaultLogger.Error(a...) }
func Warning(a ...any)                 { defaultLogger.Warning(a...) }
func Notice(a ...any)                  { defaultLogger.Notice(a...) }
func Info(a ...any)                    { defaultLogger.Info(a...) }
func Debug(a ...any)                   { defaultLogger.Debug(a...) }
func Errorf(format string, a ...any)   { defaultLogger.Errorf(format, a...) }
func Warningf(format string, a ...any) { defaultLogger.Warningf(format, a...) }
func Noticef(format string, a ...any)  { defaultLogger.Noticef(format, a...) }
func Infof(format string, a ...any)    { defaultLogger.Infof(format, a...) }
func Debugf(format string, a ...any)   { defaultLogger.Debugf(format, a...) }
func With(args ...any) *Logger         { return defaultLogger.With(args...) }
func When(cond bool) ConditionalLogger { return defaultLogger.When(cond) }
func Once(key string) LimitedLogger    { return defaultLogger.Once(key) }
func Limit(key string, n int, d time.Duration) LimitedLogger {
	return defaultLogger.Limit(key, n, d)
}
func ResetAllOnce() { defaultLogger.ResetAllOnce() }
