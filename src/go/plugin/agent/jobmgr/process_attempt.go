// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
)

var (
	ErrProcessAttemptBusy        = errors.New("jobmgr containment: identity busy")
	ErrProcessAttemptDeadline    = errors.New("jobmgr containment: attempt crossed the containment fuse")
	ErrProcessAttemptSuperseded  = errors.New("jobmgr containment: attempt superseded")
	ErrProcessAttemptRetired     = errors.New("jobmgr containment: target generation retired")
	ErrProcessAttemptStopped     = errors.New("jobmgr containment: process authority stopped")
	ErrProcessAttemptSettled     = errors.New("jobmgr containment: attempt already settled")
	ErrProcessAttemptWorkerPanic = errors.New("jobmgr containment: worker panic")
)

type ProcessAttemptNamespace uint8

const (
	ProcessAttemptJob ProcessAttemptNamespace = iota + 1
	ProcessAttemptJobRuntime
	ProcessAttemptJobTest
	ProcessAttemptStore
	ProcessAttemptStoreTest
	ProcessAttemptFunctionBundle
	ProcessAttemptFunctionPoll
	ProcessAttemptFunctionInvocation
	ProcessAttemptServiceDiscovery
)

type ProcessAttemptIdentity struct {
	Namespace ProcessAttemptNamespace
	Key       string
	Resource  string
}

type ProcessAttemptPlan struct {
	Identity ProcessAttemptIdentity
	Target   uint64
	Work     func(context.Context) error
}

type ProcessAttempt interface {
	Admit() error
	Cut(error) bool
	Await(context.Context) error
	Released() <-chan struct{}
}

// ProcessAttemptAuthority is the process-lifetime capability supplied to
// source-specific staging adapters.
type ProcessAttemptAuthority interface {
	StartProcessAttempt(ProcessAttemptPlan) (ProcessAttempt, error)
	SupersedeProcessAttempt(context.Context, ProcessAttemptIdentity) error
	CutProcessAttempt(ProcessAttemptIdentity, error) bool
	ProcessAttemptReleased(ProcessAttemptIdentity) (<-chan struct{}, bool)
}
