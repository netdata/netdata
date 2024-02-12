// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"log/slog"
	"strings"
)

var Level = &level{lvl: &slog.LevelVar{}}

type level struct {
	lvl *slog.LevelVar
}

func (l *level) Enabled(level slog.Level) bool {
	return level >= l.lvl.Level()
}

func (l *level) Set(level slog.Level) {
	l.lvl.Set(level)
}

func (l *level) SetByName(level string) {
	switch strings.ToLower(level) {
	case "err", "error":
		l.lvl.Set(slog.LevelError)
	case "warn", "warning":
		l.lvl.Set(slog.LevelWarn)
	case "info":
		l.lvl.Set(slog.LevelInfo)
	case "debug":
		l.lvl.Set(slog.LevelDebug)
	}
}
