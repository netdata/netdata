// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import "time"

// WithRetryNotBefore adds an absolute lower bound for an otherwise eligible
// automatic startup retry, including source promotion of the retained candidate.
// It does not enable retries or classify the error.
// Capture the deadline when the provider requests a wait, rather than when the
// framework later disposes the failed collector. Zero deadlines are ignored.
// The original diagnostic text and error chain are preserved; nil stays nil.
func WithRetryNotBefore(err error, deadline time.Time) error {
	if err == nil || deadline.IsZero() || !RetryNotBefore(err).Before(deadline) {
		return err
	}
	return &retryNotBeforeError{
		cause:    err,
		deadline: deadline,
	}
}

// RetryNotBefore returns the latest annotated deadline in a wrapped or joined
// error tree, or zero when none is present. Reading it never restarts the wait.
func RetryNotBefore(err error) time.Time {
	var deadline time.Time
	walkErrorTree(err, func(err error) bool {
		if annotated, ok := err.(*retryNotBeforeError); ok && deadline.Before(annotated.deadline) {
			deadline = annotated.deadline
		}
		return true
	})
	return deadline
}

type retryNotBeforeError struct {
	cause    error
	deadline time.Time
}

func (e *retryNotBeforeError) Error() string { return e.cause.Error() }
func (e *retryNotBeforeError) Unwrap() error { return e.cause }
