// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type jobConfigPreparationError struct {
	cause   error
	failure collectorapi.JobConfigFailure
}

func (e *jobConfigPreparationError) Error() string { return e.cause.Error() }
func (e *jobConfigPreparationError) Unwrap() error { return e.cause }

func withJobConfigFailure(err error, stage, reason string) error {
	if err == nil {
		return nil
	}
	f := jobConfigFailure(err, stage)
	if reason != "" {
		f.Reason = reason
	}
	return &jobConfigPreparationError{cause: err, failure: f}
}

func jobConfigFailure(err error, stage string) collectorapi.JobConfigFailure {
	var annotated *jobConfigPreparationError
	if errors.As(err, &annotated) {
		return annotated.failure
	}
	f := collectorapi.JobConfigFailure{Stage: stage, Reason: "unknown", CompletedAt: time.Now()}
	var atomic *secretresolver.AtomicResolveError
	var invalid *invalidJobConfigurationError
	switch {
	case errors.As(err, &atomic):
		f.Stage = "secret_resolution"
		switch atomic.Kind {
		case secretresolver.AtomicErrorCycle:
			f.Reason = "cycle"
		case secretresolver.AtomicErrorDepth:
			f.Reason = "depth"
		case secretresolver.AtomicErrorResultLimit:
			f.Reason = "result_limit"
		case secretresolver.AtomicErrorReference:
			f.Reason = "reference"
		case secretresolver.AtomicErrorProvider:
			f.Reason = "provider"
		case secretresolver.AtomicErrorScope:
			f.Reason = "scope"
		case secretresolver.AtomicErrorShape:
			f.Reason = "shape"
		}
	case errors.Is(err, lifecycle.ErrTaskPanic):
		f.Reason = "panic"
	case errors.Is(err, context.Canceled):
		f.Reason = "cancelled"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, jobmgr.ErrProcessAttemptDeadline):
		f.Reason = "deadline"
	case errors.Is(err, jobmgr.ErrProcessAttemptQuarantined):
		f.Stage = "activation"
		f.Reason = "quarantined"
	case errors.Is(err, jobmgr.ErrProcessAttemptBusy):
		f.Stage = "activation"
		f.Reason = "busy"
	case errors.Is(err, jobmgr.ErrProcessAttemptSuperseded):
		f.Stage = "activation"
		f.Reason = "superseded"
	case errors.Is(err, ErrStaleStoreGeneration):
		f.Stage = "activation"
		f.Reason = "stale_secret_generation"
	case errors.As(err, &invalid):
		f.Stage = "configuration"
		f.Reason = "invalid_configuration"
	}
	return f
}
