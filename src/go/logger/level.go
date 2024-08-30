// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"log/slog"
	"strings"
)

const (
	levelNotice  = slog.Level(2)
	levelDisable = slog.Level(99)
)

var (
	customLevels = map[slog.Leveler]string{
		levelNotice: "NOTICE",
	}
	customLevelsTerm = map[slog.Leveler]string{
		levelNotice: "\u001B[34m" + "NTC" + "\u001B[0m",
	}
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
	// https://github.com/netdata/netdata/tree/master/src/libnetdata/log#log-levels
	switch strings.ToLower(level) {
	case "err", "error":
		l.lvl.Set(slog.LevelError)
	case "warn", "warning":
		l.lvl.Set(slog.LevelWarn)
	case "notice":
		l.lvl.Set(levelNotice)
	case "info":
		l.lvl.Set(slog.LevelInfo)
	case "debug":
		l.lvl.Set(slog.LevelDebug)
	case "emergency", "alert", "critical":
		l.lvl.Set(levelDisable)
	}
}
