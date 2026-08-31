// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

var errResolvedLifecycleRedacted = errors.New(
	"job output: collector lifecycle failed; resolved configuration details redacted",
)

type redactedResolvedProcessControlError struct {
	cause error
}

func (err *redactedResolvedProcessControlError) Error() string {
	return errResolvedLifecycleRedacted.Error()
}

func (err *redactedResolvedProcessControlError) Unwrap() error {
	return err.cause
}

type redactedResolvedCodedError struct {
	cause     error
	code      int
	retryable bool
}

func (err *redactedResolvedCodedError) Error() string         { return err.cause.Error() }
func (err *redactedResolvedCodedError) Unwrap() error         { return err.cause }
func (err *redactedResolvedCodedError) DyncfgCode() int       { return err.code }
func (err *redactedResolvedCodedError) DyncfgRetryable() bool { return err.retryable }

type redactedResolvedRetryableError struct {
	cause error
}

func (err *redactedResolvedRetryableError) Error() string         { return err.cause.Error() }
func (err *redactedResolvedRetryableError) Unwrap() error         { return err.cause }
func (err *redactedResolvedRetryableError) DyncfgRetryable() bool { return true }

func redactResolvedLifecycleError(err error) error {
	if err == nil {
		return nil
	}
	safe := error(errResolvedLifecycleRedacted)
	if jobmgr.ContainsOnlyErrorLeaves(
		err,
		jobmgr.ErrProcessAttemptRetired,
		jobmgr.ErrProcessAttemptStopped,
	) {
		retired := errors.Is(err, jobmgr.ErrProcessAttemptRetired)
		stopped := errors.Is(err, jobmgr.ErrProcessAttemptStopped)
		var cause error
		switch {
		case retired && stopped:
			cause = errors.Join(
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			)
		case retired:
			cause = jobmgr.ErrProcessAttemptRetired
		case stopped:
			cause = jobmgr.ErrProcessAttemptStopped
		}
		if cause != nil {
			safe = &redactedResolvedProcessControlError{cause: cause}
		}
	}
	if errors.Is(err, context.Canceled) {
		safe = errors.Join(safe, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		safe = errors.Join(safe, context.DeadlineExceeded)
	}
	if errors.Is(err, lifecycle.ErrTaskPanic) {
		safe = errors.Join(safe, lifecycle.ErrTaskPanic)
	}
	var resolveErr *secretresolver.AtomicResolveError
	if errors.As(err, &resolveErr) {
		safe = &secretresolver.AtomicResolveError{Kind: resolveErr.Kind, Cause: safe}
	}
	if coded, ok := errors.AsType[dyncfg.CodedError](err); ok {
		safe = &redactedResolvedCodedError{
			cause:     safe,
			code:      coded.DyncfgCode(),
			retryable: dyncfg.IsRetryableError(err),
		}
	} else if dyncfg.IsRetryableError(err) {
		safe = &redactedResolvedRetryableError{cause: safe}
	}
	var invalid *invalidJobConfigurationError
	if errors.As(err, &invalid) {
		safe = invalidJobConfiguration(safe)
	}
	var transient *transientJobConstructionError
	if errors.As(err, &transient) {
		safe = transientJobConstruction(safe)
	}
	if lifecycle.OwnershipRetained(err) {
		safe = lifecycle.RetainOwnership(safe)
	}
	return safe
}
