package logger

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/lmittmann/tint"
)

func newTextHandler() slog.Handler {
	return slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: Level.lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				if isJournal {
					return slog.Attr{}
				}
			case slog.LevelKey:
				lvl := a.Value.Any().(slog.Level)
				s, ok := customLevels[lvl]
				if !ok {
					s = lvl.String()
				}
				return slog.String(a.Key, strings.ToLower(s))
			}
			return a
		},
	})
}

func newTerminalHandler() slog.Handler {
	return tint.NewHandler(os.Stderr, &tint.Options{
		NoColor:   runtime.GOOS == "windows",
		AddSource: true,
		Level:     Level.lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				return slog.Attr{}
			case slog.SourceKey:
				if !Level.Enabled(slog.LevelDebug) {
					return slog.Attr{}
				}
			case slog.LevelKey:
				lvl := a.Value.Any().(slog.Level)
				if s, ok := customLevelsTerm[lvl]; ok {
					return slog.String(a.Key, s)
				}
			}
			return a
		},
	})
}

func withCallDepth(depth int, sh slog.Handler) slog.Handler {
	if v, ok := sh.(*callDepthHandler); ok {
		sh = v.sh
	}
	return &callDepthHandler{depth: depth, sh: sh}
}

type callDepthHandler struct {
	depth int
	sh    slog.Handler
}

func (h *callDepthHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.sh.Enabled(ctx, level)
}

func (h *callDepthHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return withCallDepth(h.depth, h.sh.WithAttrs(attrs))
}

func (h *callDepthHandler) WithGroup(name string) slog.Handler {
	return withCallDepth(h.depth, h.sh.WithGroup(name))
}

func (h *callDepthHandler) Handle(ctx context.Context, r slog.Record) error {
	// https://pkg.go.dev/log/slog#example-package-Wrapping
	var pcs [1]uintptr
	// skip Callers and this function
	runtime.Callers(h.depth+2, pcs[:])
	r.PC = pcs[0]

	return h.sh.Handle(ctx, r)
}
