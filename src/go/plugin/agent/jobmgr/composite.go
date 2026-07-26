// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

// ErrCompositeRecoveryUnresolved reports recovery work that did not reach a
// terminal ownership state after its bounded context was cancelled.
var ErrCompositeRecoveryUnresolved = errors.New("jobmgr composite: recovery child remained unresolved after cancellation")

// PreparedCompositeResourceTransaction is a resource transaction whose Apply
// phase may execute acknowledged child commands inside the parent's ordering
// and claim scope.
type PreparedCompositeResourceTransaction interface {
	Scope() lifecycle.ResourceTransactionScope
	ApplyComposite(context.Context, CompositeCommandScope) (lifecycle.AppliedResourceTransaction, error)
	Dispose(context.Context) (lifecycle.ReadyResource, error)
}

// CompositeResourceTransactionWork prepares one composite resource
// transaction without exposing CommandKernel itself to an adapter.
type CompositeResourceTransactionWork func(
	context.Context,
	lifecycle.ReadyResource,
	lifecycle.ResourceTransactionScope,
	lifecycle.LongLivedPermit,
) (PreparedCompositeResourceTransaction, error)

// CompositeCommandScope owns acknowledged child execution for one running
// parent transaction. SubmitPreparedAndWait follows caller/parent
// cancellation. SubmitRecoveryAndWait uses the adapter-owned bounded recovery
// context and remains eligible as an ownership continuation after cancellation.
type CompositeCommandScope interface {
	SubmitPreparedAndWait(context.Context, Request, WorkPlan) error
	SubmitRecoveryAndWait(context.Context, Request, WorkPlan) error
}
