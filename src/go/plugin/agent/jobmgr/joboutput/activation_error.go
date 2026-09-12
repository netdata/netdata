// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
)

type activationFailureKind uint8

const (
	activationFailureOperational activationFailureKind = iota
	activationFailureProposal
	activationFailureTransient
	activationFailureBusy
	activationFailureStaleStore
	activationFailureSuperseded
	activationFailureDeadline
	activationFailureQuarantined
)

// activationFailure preserves the cause while giving every activation path the
// same failure facts. Callers own cancellation, graph disposition and recovery.
type activationFailure struct {
	err  error
	kind activationFailureKind
}

func classifyActivationError(err error) activationFailure {
	if err == nil {
		return activationFailure{}
	}
	failure := activationFailure{
		err: err,
	}
	var transient *transientJobConstructionError
	var resolveErr *secretresolver.AtomicResolveError
	var invalid *invalidJobConfigurationError
	switch {
	case errors.Is(err, jobmgr.ErrProcessAttemptQuarantined):
		failure.kind = activationFailureQuarantined
	case errors.Is(err, jobmgr.ErrProcessAttemptBusy):
		failure.kind = activationFailureBusy
	case errors.Is(err, ErrStaleStoreGeneration):
		failure.kind = activationFailureStaleStore
	case errors.Is(err, jobmgr.ErrProcessAttemptSuperseded):
		failure.kind = activationFailureSuperseded
	case errors.Is(err, jobmgr.ErrProcessAttemptDeadline):
		// The containment fuse has its own recovery policy. Callers handle context
		// expiry; provider/scope deadlines keep their transient classification below.
		failure.kind = activationFailureDeadline
	case errors.As(err, &transient):
		failure.kind = activationFailureTransient
	case errors.As(err, &resolveErr):
		switch resolveErr.Kind {
		case secretresolver.AtomicErrorProvider, secretresolver.AtomicErrorScope:
			failure.kind = activationFailureTransient
		default:
			failure.kind = activationFailureProposal
		}
	case errors.As(err, &invalid):
		failure.kind = activationFailureProposal
	}
	return failure
}
