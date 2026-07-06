// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"runtime/debug"
)

// LaneKeyDeriver narrows a registration's schedule lane per request. The
// scheduler serializes requests that share a schedule key until each reaches
// its terminal finalize; without a deriver every request routed to one
// registration shares one lane, so an expensive command blocks unrelated ones
// behind it. A deriver returns the identity the request addresses (for dyncfg,
// the config a command targets) so only same-identity requests serialize.
//
// Returning laneKey == "" selects the registration-wide lane (the behavior
// without a deriver) - the fallback for requests whose identity cannot be
// derived; such requests still reach the handler's own rejection paths. meta,
// when non-nil, travels with the request and is retrievable by the handler
// via LaneMetaFromContext.
//
// Derivers run on the dispatch path and must be side-effect-free and safe to
// call from the manager goroutine. A panic is contained: the request falls
// back to the registration-wide lane.
type LaneKeyDeriver func(fn Function) (laneKey string, meta any)

type laneMetaCtxKey struct{}

// LaneMetaFromContext returns the metadata attached by the registration's
// LaneKeyDeriver, or nil.
func LaneMetaFromContext(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	return ctx.Value(laneMetaCtxKey{})
}

// deriveLaneKey runs derive with panic containment.
func (m *Manager) deriveLaneKey(derive LaneKeyDeriver, fn Function) (laneKey string, meta any) {
	defer func() {
		if v := recover(); v != nil {
			m.Errorf("lane key deriver panic (function '%s', uid=%s): %v\n%s", fn.Name, fn.UID, v, string(debug.Stack()))
			laneKey, meta = "", nil
		}
	}()
	return derive(fn)
}
