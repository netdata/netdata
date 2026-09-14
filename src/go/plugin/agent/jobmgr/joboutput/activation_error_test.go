// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/stretchr/testify/require"
)

func TestActivationFailureClassificationPreservesRecoveryDistinctions(t *testing.T) {
	// Contention waits for physical release; provider failures retry on a timer;
	// invalid proposals and quarantined identities must not gain such retries.
	tests := []struct {
		name string
		err  error
		kind activationFailureKind
	}{
		{name: "success", kind: activationFailureOperational},
		{name: "operational", err: errors.New("construction failed"), kind: activationFailureOperational},
		{
			name: "invalid config",
			err:  invalidJobConfiguration(errors.New("invalid shape")),
			kind: activationFailureProposal,
		},
		{
			name: "transient construction",
			err:  transientJobConstruction(errors.New("dependency unavailable")),
			kind: activationFailureTransient,
		},
		{
			name: "provider",
			err: &secretresolver.AtomicResolveError{
				Kind: secretresolver.AtomicErrorProvider,
			},
			kind: activationFailureTransient,
		},
		{
			name: "scope",
			err: &secretresolver.AtomicResolveError{
				Kind: secretresolver.AtomicErrorScope,
			},
			kind: activationFailureTransient,
		},
		{
			name: "invalid reference",
			err: &secretresolver.AtomicResolveError{
				Kind: secretresolver.AtomicErrorReference,
			},
			kind: activationFailureProposal,
		},
		{
			name: "resolution limit",
			err: &secretresolver.AtomicResolveError{
				Kind: secretresolver.AtomicErrorResultLimit,
			},
			kind: activationFailureProposal,
		},
		{name: "busy", err: jobmgr.ErrProcessAttemptBusy, kind: activationFailureBusy},
		{name: "stale Store", err: ErrStaleStoreGeneration, kind: activationFailureStaleStore},
		{name: "superseded", err: jobmgr.ErrProcessAttemptSuperseded, kind: activationFailureSuperseded},
		{name: "deadline", err: jobmgr.ErrProcessAttemptDeadline, kind: activationFailureDeadline},
		{name: "quarantined", err: jobmgr.ErrProcessAttemptQuarantined, kind: activationFailureQuarantined},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.err
			if err != nil {
				err = fmt.Errorf("candidate: %w", err)
			}
			failure := classifyActivationError(err)
			require.Equal(t, test.kind, failure.kind)
			require.Equal(t, err, failure.err)
		})
	}
}

func TestActivationFailureKeepsContainmentAndOwnershipAuthoritative(t *testing.T) {
	err := lifecycle.RetainOwnership(errors.Join(
		jobmgr.ErrProcessAttemptQuarantined,
		transientJobConstruction(jobmgr.ErrProcessAttemptBusy),
	))
	failure := classifyActivationError(err)
	require.Equal(t, activationFailureQuarantined, failure.kind)
	require.True(t, lifecycle.OwnershipRetained(failure.err))
	require.ErrorIs(t, failure.err, jobmgr.ErrProcessAttemptBusy)

	// A provider may cancel its own work while the command remains live. Keep
	// that transient fact; only the caller can decide its lifetime disposition.
	err = &secretresolver.AtomicResolveError{
		Kind:  secretresolver.AtomicErrorProvider,
		Cause: context.Canceled,
	}
	failure = classifyActivationError(err)
	require.Equal(t, activationFailureTransient, failure.kind)
	require.ErrorIs(t, failure.err, context.Canceled)
}
