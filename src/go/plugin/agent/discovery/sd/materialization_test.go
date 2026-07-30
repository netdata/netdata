// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/stretchr/testify/require"
)

func TestMaterializationIdentityStructurallyEncodesValues(t *testing.T) {
	require.NotEqual(
		t,
		materializationIdentity("userconfig", []byte("fixture"), []byte("a\x00b"), []byte("c")),
		materializationIdentity("userconfig", []byte("fixture"), []byte("a"), []byte("b\x00c")),
	)
}

func TestQuarantinedMaterializationIsAConfigLocalFailure(t *testing.T) {
	identity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptServiceDiscovery,
		Key:       "pipeline",
		Resource:  "service discovery configuration",
	}
	err := classifyMaterializationError(jobmgr.ErrProcessAttemptQuarantined, identity)

	var contained *materializationError
	require.True(t, errors.As(err, &contained))
	require.Equal(t, identity, contained.identity)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)
	require.Equal(t, 503, contained.DyncfgCode())
}

func TestMaterializationDoesNotSupersedeOrStartAfterCallerCancellation(t *testing.T) {
	attempts := &canceledMaterializationAuthority{}
	discovery := &ServiceDiscovery{
		epoch:    1,
		attempts: attempts,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32

	_, err := runMaterialization(
		ctx,
		discovery,
		"materialization",
		true,
		func(context.Context) (struct{}, error) {
			calls.Add(1)
			return struct{}{}, nil
		},
	)

	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, attempts.superseded.Load())
	require.Zero(t, attempts.started.Load())
	require.Zero(t, calls.Load())
}

func TestMaterializationDoesNotStartWhenCanceledDuringSupersession(t *testing.T) {
	attempts := newCancelDuringSupersessionAuthority()
	discovery := &ServiceDiscovery{
		epoch:    1,
		attempts: attempts,
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		_, err := runMaterialization(
			ctx,
			discovery,
			"materialization",
			true,
			func(context.Context) (struct{}, error) {
				return struct{}{}, nil
			},
		)
		result <- err
	}()

	supersedeTimer := time.NewTimer(time.Second)
	defer supersedeTimer.Stop()
	select {
	case <-attempts.supersedeEntered:
	case <-supersedeTimer.C:
		t.Fatal("materialization did not enter supersession")
	}
	cancel()
	close(attempts.supersedeRelease)

	resultTimer := time.NewTimer(time.Second)
	defer resultTimer.Stop()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-resultTimer.C:
		t.Fatal("materialization did not return after cancellation")
	}
	require.Zero(t, attempts.started.Load())
}

type canceledMaterializationAuthority struct {
	started    atomic.Int32
	superseded atomic.Int32
}

func (a *canceledMaterializationAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	a.started.Add(1)
	return nil, errors.New("test: canceled materialization started")
}

func (a *canceledMaterializationAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	a.superseded.Add(1)
	return errors.New("test: canceled materialization superseded")
}

func (*canceledMaterializationAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (*canceledMaterializationAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return nil, false
}

type cancelDuringSupersessionAuthority struct {
	started          atomic.Int32
	supersedeEntered chan struct{}
	supersedeRelease chan struct{}
}

func newCancelDuringSupersessionAuthority() *cancelDuringSupersessionAuthority {
	return &cancelDuringSupersessionAuthority{
		supersedeEntered: make(chan struct{}),
		supersedeRelease: make(chan struct{}),
	}
}

func (a *cancelDuringSupersessionAuthority) StartProcessAttempt(
	ctx context.Context,
	_ jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	a.started.Add(1)
	return nil, errors.New("test: canceled materialization started")
}

func (a *cancelDuringSupersessionAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	close(a.supersedeEntered)
	<-a.supersedeRelease
	return nil
}

func (*cancelDuringSupersessionAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (*cancelDuringSupersessionAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return nil, false
}
