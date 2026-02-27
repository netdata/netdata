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
		return &callDepthHandler{depth: depth, isTerminal: v.isTerminal, sh: sh}
	}
	return &callDepthHandler{depth: depth, sh: sh}
}

func withTerminalCallDepth(depth int, sh slog.Handler) slog.Handler {
	if v, ok := sh.(*callDepthHandler); ok {
		sh = v.sh
	}
	return &callDepthHandler{depth: depth, isTerminal: true, sh: sh}
}

type callDepthHandler struct {
	depth      int
	isTerminal bool
	sh         slog.Handler
}

func (h *callDepthHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.sh.Enabled(ctx, level)
}

func (h *callDepthHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h.isTerminal {
		return withTerminalCallDepth(h.depth, h.sh.WithAttrs(attrs))
	}
	return withCallDepth(h.depth, h.sh.WithAttrs(attrs))
}

func (h *callDepthHandler) WithGroup(name string) slog.Handler {
	if h.isTerminal {
		return withTerminalCallDepth(h.depth, h.sh.WithGroup(name))
	}
	return withCallDepth(h.depth, h.sh.WithGroup(name))
}

func (h *callDepthHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.isTerminal && Level.Enabled(slog.LevelDebug) {
		// Keep fixed-skip math identical to the non-dynamic path.
		// resolveCallerPC will adjust for its own extra stack frame on fallback.
		r.PC = resolveCallerPC(h.depth + 2)
		return h.sh.Handle(ctx, r)
	}

	// https://pkg.go.dev/log/slog#example-package-Wrapping
	var pcs [1]uintptr
	// skip Callers and this function
	runtime.Callers(h.depth+2, pcs[:])
	r.PC = pcs[0]

	return h.sh.Handle(ctx, r)
}

var callerSkipPrefixes = []string{
	"github.com/netdata/netdata/go/plugins/logger.",
	"log/slog.",
	"runtime.",
}

func resolveCallerPC(fixedSkip int) uintptr {
	var pcs [15]uintptr
	n := runtime.Callers(2, pcs[:]) // skip runtime.Callers + resolveCallerPC
	if n > 0 {
		frames := runtime.CallersFrames(pcs[:n])
		for {
			frame, more := frames.Next()
			if !isSkippedCaller(frame.Function) {
				return frame.PC
			}
			if !more {
				break
			}
		}
	}

	var fallback [1]uintptr
	// fixedSkip is the skip value used by Handle's fixed path.
	// Add one extra frame to account for resolveCallerPC itself.
	runtime.Callers(fixedSkip+1, fallback[:])
	return fallback[0]
}

func isSkippedCaller(function string) bool {
	for _, prefix := range callerSkipPrefixes {
		if strings.HasPrefix(function, prefix) {
			return true
		}
	}
	return false
}
