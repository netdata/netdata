// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/stretchr/testify/require"
)

type releaseErrorAtomicScope struct {
	err error
}

func (reas releaseErrorAtomicScope) Resolve(context.Context, string, string) ([]byte, error) {
	return nil, nil
}

func (reas releaseErrorAtomicScope) Release(context.Context) error {
	return reas.err
}

func TestProcessOwnedAtomicScopeReportsReleaseFailureWithoutRunOwnership(t *testing.T) {
	releaseErr := errors.New("store scope release failed")
	diagnostics := &recordingCompositionDiagnosticObserver{}
	scope := &processOwnedAtomicScope{
		generation:  7,
		diagnostics: diagnostics,
		scope: releaseErrorAtomicScope{
			err: releaseErr,
		},
	}

	require.ErrorIs(t, scope.Release(t.Context()), releaseErr)
	require.Nil(t, scope.scope)

	events := diagnostics.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, jobmgr.DiagnosticError, events[0].Level)
	require.Equal(t, "secret Store scope release failed", events[0].Name)
	require.EqualValues(t, 7, events[0].Generation)
	require.ErrorIs(t, events[0].Err, releaseErr)
}
