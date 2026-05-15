// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import "context"

type contextKey struct{}

// ContextWithLogger attaches a logger to a context.
func ContextWithLogger(ctx context.Context, log *Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if log == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, log)
}

// LoggerFromContext returns a logger attached to a context.
func LoggerFromContext(ctx context.Context) (*Logger, bool) {
	if ctx == nil {
		return nil, false
	}
	log, ok := ctx.Value(contextKey{}).(*Logger)
	return log, ok && log != nil
}
