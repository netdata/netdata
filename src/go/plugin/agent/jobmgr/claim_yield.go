// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
)

type claimYieldLease interface {
	release(context.Context) error
	reacquire(context.Context) error
}

type claimYieldContextKey struct{}

func withClaimYieldLease(ctx context.Context, lease claimYieldLease) context.Context {
	return context.WithValue(ctx, claimYieldContextKey{}, lease)
}

// RunWithoutClaims temporarily releases the current transaction's configured
// acquisition-suffix claim while work runs, then reacquires it before returning.
func RunWithoutClaims(ctx context.Context, work func(context.Context) error) (workErr, claimErr error) {
	if ctx == nil || work == nil {
		return nil, errors.New("jobmgr claims: invalid yielded work")
	}
	lease, ok := ctx.Value(claimYieldContextKey{}).(claimYieldLease)
	if !ok || lease == nil {
		return nil, errors.New("jobmgr claims: yielded work is unavailable")
	}
	if err := lease.release(ctx); err != nil {
		return nil, err
	}
	defer func() {
		claimErr = lease.reacquire(context.WithoutCancel(ctx))
	}()
	workErr = work(ctx)
	return workErr, claimErr
}
