// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type releaseErrorAtomicScope struct {
	err error
}

func (scope releaseErrorAtomicScope) Resolve(
	context.Context,
	string,
	string,
) ([]byte, error) {
	return nil, nil
}

func (scope releaseErrorAtomicScope) Release(
	context.Context,
) error {
	return scope.err
}

func TestRunOwnedAtomicScopeDirtiesRunOnReleaseFailure(
	t *testing.T,
) {
	releaseErr := errors.New("Store scope release failed")
	run, err := lifecycle.NewRunSupervisor(
		1,
		lifecycle.RealClock{},
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	scope := &runOwnedAtomicScope{
		run: run,
		scope: releaseErrorAtomicScope{
			err: releaseErr,
		},
	}
	if err := scope.Release(t.Context()); !errors.Is(
		err,
		releaseErr,
	) {
		t.Fatalf("release error=%v want=%v", err, releaseErr)
	}
	if !errors.Is(run.DirtyCause(), releaseErr) {
		t.Fatalf(
			"run dirty cause=%v want=%v",
			run.DirtyCause(),
			releaseErr,
		)
	}
}
