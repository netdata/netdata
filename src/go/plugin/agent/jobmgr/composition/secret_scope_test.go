// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

type releaseErrorAtomicScope struct {
	err error
}

func (reas releaseErrorAtomicScope) Resolve(
	context.Context,
	string,
	string,
) ([]byte, error) {
	return nil, nil
}

func (reas releaseErrorAtomicScope) Release(
	context.Context,
) error {
	return reas.err
}

func TestRunOwnedAtomicScopeDirtiesRunOnReleaseFailure(
	t *testing.T,
) {
	releaseErr := errors.New("store scope release failed")
	run, err := lifecycle.NewRunSupervisor(
		1,
		lifecycle.RealClock{},
		time.Second,
	)
	require.NoError(t, err)
	scope := &runOwnedAtomicScope{
		run: run,
		scope: releaseErrorAtomicScope{
			err: releaseErr,
		},
	}

	require.ErrorIs(t, scope.Release(t.Context()), releaseErr)

	require.ErrorIs(t, run.DirtyCause(), releaseErr)
}
