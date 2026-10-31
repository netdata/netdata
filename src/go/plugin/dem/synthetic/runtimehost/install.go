// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type Consumer interface {
	Manifest() runtime.Manifest
	BeginUse() (*runtime.Use, error)
}

// Borrow exposes a caller-owned process or candidate claim to the runner. It
// does not acquire or release that claim; the caller owns its full lifetime.
func Borrow(consumer Consumer) runner.Source { return borrowedSource{consumer} }

type borrowedSource struct{ consumer Consumer }

func (s borrowedSource) BeginUse() (runner.Use, error) {
	use, err := s.consumer.BeginUse()
	if err != nil {
		return nil, err
	}
	m := s.consumer.Manifest()
	return managedUse{use: use, identity: synthetic.RuntimeIdentity{Version: m.Version, SHA256: m.Archive.SHA256, Contract: m.Contract}}, nil
}

type Qualifier interface {
	Qualify(context.Context, runner.Source) (*runner.Qualification, error)
}

// PrepareRelease owns one complete managed transaction. Qualification must use
// the process's existing execution owner when called inside a running plugin.
// A nil result with no error means no candidate needed preparation.
func PrepareRelease(ctx context.Context, store *runtime.Store, options runtime.ReleaseInstallOptions,
	load func(context.Context) (runtime.Release, error), archive string, qualifier Qualifier) (_ *runner.Qualification, result error) {
	if qualifier == nil {
		return nil, errors.New("runtime qualifier is unavailable")
	}
	candidate, err := store.BeginPublishedInstall(ctx, options, load)
	if err != nil || candidate == nil {
		return nil, err
	}
	defer func() {
		if !errors.Is(result, ndexec.ErrTreeNotDrained) {
			result = errors.Join(result, candidate.Close())
		}
	}()
	if archive == "" {
		err = candidate.Download(ctx)
	} else {
		err = candidate.Import(ctx, archive)
	}
	if err != nil {
		return nil, err
	}
	qualification, err := qualifier.Qualify(ctx, Borrow(candidate))
	if err != nil {
		return qualification, err
	}
	return qualification, candidate.Select()
}
