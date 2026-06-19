// SPDX-License-Identifier: GPL-3.0-or-later

package runtimecomp

import "context"

type contextKey struct{}

// ContextWithService attaches a runtime component service to a context.
func ContextWithService(ctx context.Context, svc Service) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if svc == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, svc)
}

// ServiceFromContext returns a runtime component service from a context.
func ServiceFromContext(ctx context.Context) (Service, bool) {
	if ctx == nil {
		return nil, false
	}
	svc, ok := ctx.Value(contextKey{}).(Service)
	return svc, ok && svc != nil
}
