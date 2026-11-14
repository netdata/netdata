// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
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
	return managedUse{use: use, identity: s.consumer.Manifest().Identity()}, nil
}

// managedUse is one execution pin on a held claim. The process source records
// failed retirement as host maintenance; borrowed sources leave it to their owner.
type managedUse struct {
	use      *runtime.Use
	host     *Host
	identity synthetic.RuntimeIdentity
}

func (u managedUse) Paths() runner.Paths                { return runnerPaths(u.use.Paths, u.identity) }
func (u managedUse) Completion() *ndexec.TreeCompletion { return u.use.Completion }
func (u managedUse) Finish(drained bool) error {
	err := u.use.Finish(drained)
	if err != nil && u.host != nil {
		u.host.recordFinishError(err)
	}
	return err
}

// unavailableSource reports why the process has no usable runtime.
type unavailableSource struct{ err error }

func (s unavailableSource) BeginUse() (runner.Use, error) { return nil, s.err }

// A verified preload remains owned by the administrator, outside managed GC.
type preloadedSource struct{ paths runner.Paths }

func (s preloadedSource) BeginUse() (runner.Use, error) { return preloadedUse(s), nil }

type preloadedUse struct{ paths runner.Paths }

func (u preloadedUse) Paths() runner.Paths                { return u.paths }
func (u preloadedUse) Completion() *ndexec.TreeCompletion { return nil }
func (u preloadedUse) Finish(bool) error                  { return nil }

func runnerPaths(paths runtime.Paths, identity synthetic.RuntimeIdentity) runner.Paths {
	return runner.Paths{Node: paths.Node, EntryPoint: paths.EntryPoint, Identity: identity}
}
