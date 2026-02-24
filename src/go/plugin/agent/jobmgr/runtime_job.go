// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// runtimeJob is the minimal job runner contract needed by jobmgr orchestration.
// It is implemented by both legacy module.Job and module.JobV2.
type runtimeJob interface {
	FullName() string
	ModuleName() string
	Name() string
	Collector() any

	Start()
	Stop()
	Tick(clock int)

	AutoDetection() error
	AutoDetectionEvery() int
	RetryAutoDetection() bool
	Cleanup()

	IsRunning() bool
	Panicked() bool

	Vnode() vnodes.VirtualNode
	UpdateVnode(vnode *vnodes.VirtualNode)
}

// configModule is the shared Init/Check/config contract for dyncfg config test/get
// flows. Both legacy CollectorV1 and CollectorV2 satisfy it.
type configModule interface {
	GetBase() *collectorapi.Base
	Init(ctx context.Context) error
	Check(ctx context.Context) error
	Cleanup(ctx context.Context)
	Configuration() any
}
