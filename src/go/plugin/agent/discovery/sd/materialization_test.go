// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

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

type canceledMaterializationAuthority struct {
	started    atomic.Int32
	superseded atomic.Int32
}

func (a *canceledMaterializationAuthority) StartProcessAttempt(
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
