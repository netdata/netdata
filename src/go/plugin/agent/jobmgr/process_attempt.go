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

func (namespace ProcessAttemptNamespace) String() string {
	switch namespace {
	case ProcessAttemptJob:
		return "job"
	case ProcessAttemptJobRuntime:
		return "job-runtime"
	case ProcessAttemptJobTest:
		return "job-test"
	case ProcessAttemptStore:
		return "store"
	case ProcessAttemptStoreTest:
		return "store-test"
	case ProcessAttemptFunctionBundle:
		return "function-bundle"
	case ProcessAttemptFunctionPoll:
		return "function-poll"
	case ProcessAttemptFunctionInvocation:
		return "function-invocation"
	case ProcessAttemptServiceDiscovery:
		return "service-discovery"
	default:
		return "invalid"
	}
}

type ProcessAttemptIdentity struct {
	Namespace ProcessAttemptNamespace
	Key       string
	Resource  string
}

type ProcessAttemptPlan struct {
	Identity ProcessAttemptIdentity
	Target   uint64
	Work     func(context.Context, ProcessAttemptAdmission) error
}

// ProcessAttemptAdmission is the only attempt control available to its worker.
// Admission ends the preparation fuse without releasing physical ownership.
type ProcessAttemptAdmission interface {
	Admit() error
}

type ProcessAttempt interface {
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
