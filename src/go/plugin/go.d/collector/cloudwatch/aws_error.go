// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const maxFailureLogSamples = 4

type sanitizedAWSError struct {
	description string
	cause       error
}

func (e sanitizedAWSError) Error() string { return e.description }
func (e sanitizedAWSError) Unwrap() error { return e.cause }

// sanitizeAWSError preserves error identity for cancellation checks while
// excluding provider messages, credential details, endpoints, and role context.
func sanitizeAWSError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return sanitizedAWSError{description: "AWS request timed out", cause: err}
	case errors.Is(err, context.Canceled):
		return sanitizedAWSError{description: "AWS request canceled", cause: err}
	}

	var details []string
	var apiErr interface {
		ErrorCode() string
	}
	if errors.As(err, &apiErr) && strings.TrimSpace(apiErr.ErrorCode()) != "" {
		details = append(details, "code="+apiErr.ErrorCode())
	}
	var responseErr interface {
		HTTPStatusCode() int
		ServiceRequestID() string
	}
	if errors.As(err, &responseErr) {
		if status := responseErr.HTTPStatusCode(); status > 0 {
			details = append(details, fmt.Sprintf("status=%d", status))
		}
		if requestID := strings.TrimSpace(responseErr.ServiceRequestID()); requestID != "" {
			details = append(details, "request_id="+requestID)
		}
	}

	description := "AWS request failed"
	if len(details) > 0 {
		description += " (" + strings.Join(details, ", ") + ")"
	}
	return sanitizedAWSError{description: description, cause: err}
}

type operationFailure struct {
	Target string
	Region string
	Scope  string
	Err    error
}

func (c *Collector) warnOperationFailures(limiterKey, operation, suffix string, failures []operationFailure) {
	if len(failures) == 0 {
		return
	}
	var samples []string
	for _, failure := range failures[:min(len(failures), maxFailureLogSamples)] {
		scope := ""
		if failure.Scope != "" {
			scope = " " + failure.Scope
		}
		samples = append(samples, fmt.Sprintf("target %q region %q%s: %v", failure.Target, failure.Region, scope, sanitizeAWSError(failure.Err)))
	}
	if remaining := len(failures) - len(samples); remaining > 0 {
		samples = append(samples, fmt.Sprintf("and %d more", remaining))
	}
	c.Limit(limiterKey, 1, recurringLogEvery).Warningf(
		"CloudWatch %s failed for %d target/region operation(s): %s%s",
		operation, len(failures), strings.Join(samples, "; "), suffix,
	)
}
