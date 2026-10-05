// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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
		if resolveErr.Kind == secretresolver.AtomicErrorScope && errors.Is(err, secretstore.ErrStoreNotFound) {
			safe = errors.Join(safe, secretstore.ErrStoreNotFound)
		}
		safe = &secretresolver.AtomicResolveError{Kind: resolveErr.Kind, Cause: safe}
	}
	switch collectorapi.ClassifyLifecycleError(err) {
	case collectorapi.LifecycleErrorPermanent:
		safe = collectorapi.PermanentError(safe)
	case collectorapi.LifecycleErrorTemporary:
		safe = collectorapi.TemporaryError(safe)
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
	var preparation *jobConfigPreparationError
	if errors.As(err, &preparation) {
		safe = &jobConfigPreparationError{cause: safe, failure: preparation.failure}
	}
	if startup, ok := onlyRuntimeStartupFailure(err); ok {
		copy := *startup.failure
		copy.cause = safe
		safe = &runtimeStartupFailure{failure: &copy}
	}
	return safe
}
