// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

var (
	ErrProcessAttemptBusy        = errors.New("jobmgr containment: identity busy")
	ErrProcessAttemptDeadline    = errors.New("jobmgr containment: attempt crossed the containment fuse")
	ErrProcessAttemptSuperseded  = errors.New("jobmgr containment: attempt superseded")
	ErrProcessAttemptRetired     = errors.New("jobmgr containment: target generation retired")
	ErrProcessAttemptStopped     = errors.New("jobmgr containment: process authority stopped")
	ErrProcessAttemptSettled     = errors.New("jobmgr containment: attempt already settled")
	ErrProcessAttemptQuarantined = errors.New("jobmgr containment: identity quarantined until process restart")
	ErrProcessAttemptWorkerPanic = errors.New("jobmgr containment: worker panic")
	ErrProcessAttemptFencePanic  = errors.New("jobmgr containment: containment fence panic")
)

const (
	// MaximumProcessAttemptKeyBytes bounds process-lifetime map keys.
	MaximumProcessAttemptKeyBytes = 4 * 1024
	// MaximumProcessAttemptDiagnosticResourceBytes bounds retained log labels.
	MaximumProcessAttemptDiagnosticResourceBytes = 256
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

func (pan ProcessAttemptNamespace) String() string {
	switch pan {
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

// Valid reports whether the identity satisfies the process-containment bounds.
func (identity ProcessAttemptIdentity) Valid() bool {
	return identity.Namespace >= ProcessAttemptJob &&
		identity.Namespace <= ProcessAttemptServiceDiscovery &&
		identity.Key != "" &&
		len(identity.Key) <= MaximumProcessAttemptKeyBytes &&
		validProcessAttemptDiagnosticResource(identity.Resource)
}

// ProcessAttemptIdentityKey derives one fixed-size key from structurally
// framed semantic fields. Domain separates unrelated identity contracts that
// share a containment namespace.
func ProcessAttemptIdentityKey(domain string, fields ...string) string {
	if domain == "" {
		return ""
	}
	hash := sha256.New()
	writeProcessAttemptIdentityField(hash, domain)
	for _, field := range fields {
		writeProcessAttemptIdentityField(hash, field)
	}
	return string(hash.Sum(nil))
}

func writeProcessAttemptIdentityField(hash io.Writer, field string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(field)))
	_, _ = hash.Write(length[:])
	_, _ = io.WriteString(hash, field)
}

// ProcessAttemptDiagnosticResource retains a safe specific resource when
// possible and otherwise returns a bounded fallback suitable for diagnostics.
func ProcessAttemptDiagnosticResource(resource, fallback string) string {
	if validProcessAttemptDiagnosticResource(resource) {
		return resource
	}
	if validProcessAttemptDiagnosticResource(fallback) {
		return fallback
	}
	return "job manager resource"
}

func validProcessAttemptDiagnosticResource(resource string) bool {
	if resource == "" ||
		len(resource) > MaximumProcessAttemptDiagnosticResourceBytes ||
		!utf8.ValidString(resource) {
		return false
	}
	for _, char := range resource {
		if char < ' ' || char == 0x7f {
			return false
		}
	}
	return true
}

// ContainsOnlyErrorLeaves reports whether every leaf in err matches one of
// the allowed errors. Mixed and malformed trees fail closed.
func ContainsOnlyErrorLeaves(err error, allowed ...error) bool {
	if err == nil || len(allowed) == 0 {
		return false
	}
	return lifecycle.AllErrorLeavesMatch(err, func(current error) bool {
		for _, candidate := range allowed {
			if candidate != nil && errors.Is(current, candidate) {
				return true
			}
		}
		return false
	})
}

type ProcessAttemptPlan struct {
	Identity ProcessAttemptIdentity
	Target   uint64
	Work     func(context.Context, ProcessAttemptAdmission) error

	// OnContainment receives the raw cut cause under the attempt-authority lock.
	// It MUST perform only bounded in-memory work and MUST NOT re-enter the
	// authority or wait for a worker, I/O, or external cleanup. Panics are
	// recovered and reported through ErrProcessAttemptFencePanic.
	OnContainment func(error)
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
	StartProcessAttempt(context.Context, ProcessAttemptPlan) (ProcessAttempt, error)
	SupersedeProcessAttempt(context.Context, ProcessAttemptIdentity) error
	CutProcessAttempt(ProcessAttemptIdentity, error) bool
	ProcessAttemptReleased(ProcessAttemptIdentity) (<-chan struct{}, bool)
}
