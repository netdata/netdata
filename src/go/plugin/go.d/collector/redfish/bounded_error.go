// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	maxBoundedErrorClasses   = 12
	maxBoundedErrorText      = 256
	maxCollectionDiagnostics = 256
)

type boundedDiagnosticAccumulator struct {
	values  []string
	seen    map[string]struct{}
	omitted int
}

func (a *boundedDiagnosticAccumulator) Add(value string) {
	value = boundedDiagnostic(strings.TrimSpace(value))
	if value == "" {
		return
	}
	if _, exists := a.seen[value]; exists {
		return
	}
	if len(a.values) >= maxCollectionDiagnostics {
		a.omitted++
		return
	}
	if a.seen == nil {
		a.seen = make(map[string]struct{})
	}
	a.seen[value] = struct{}{}
	a.values = append(a.values, value)
}

func (a *boundedDiagnosticAccumulator) Values() []string {
	result := append([]string(nil), a.values...)
	if a.omitted > 0 {
		result = append(result, fmt.Sprintf(
			"%d additional Redfish collection diagnostics were omitted by the fixed internal bound",
			a.omitted,
		))
	}
	return result
}

type boundedErrorRepresentative struct {
	class string
	count int
	err   error
}

type boundedErrorAccumulator struct {
	total           int
	representatives []boundedErrorRepresentative
	byClass         map[string]int
}

func (a *boundedErrorAccumulator) Add(err error) {
	if err == nil {
		return
	}
	a.total++
	class := boundedErrorClass(err)
	if index, exists := a.byClass[class]; exists {
		a.representatives[index].count++
		return
	}
	if len(a.representatives) >= maxBoundedErrorClasses {
		return
	}
	if a.byClass == nil {
		a.byClass = make(map[string]int)
	}
	a.byClass[class] = len(a.representatives)
	a.representatives = append(a.representatives, boundedErrorRepresentative{
		class: class,
		count: 1,
		err:   err,
	})
}

func (a *boundedErrorAccumulator) Err() error {
	if a.total == 0 {
		return nil
	}
	representatives := append([]boundedErrorRepresentative(nil), a.representatives...)
	return &boundedOperationError{total: a.total, representatives: representatives}
}

func boundedErrorClass(err error) string {
	switch {
	case errors.Is(err, errIdentityIntegrity):
		return "identity"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return classifyError(err)
	}
}

type boundedOperationError struct {
	total           int
	representatives []boundedErrorRepresentative
}

func (e *boundedOperationError) Error() string {
	parts := make([]string, 0, len(e.representatives))
	for _, representative := range e.representatives {
		parts = append(parts, fmt.Sprintf(
			"%s (%d): %s",
			representative.class,
			representative.count,
			boundedErrorText(representative.err),
		))
	}
	return fmt.Sprintf(
		"%d Redfish operation failures; representative failures: %s",
		e.total,
		strings.Join(parts, "; "),
	)
}

func (e *boundedOperationError) Unwrap() []error {
	result := make([]error, 0, len(e.representatives))
	for _, representative := range e.representatives {
		result = append(result, representative.err)
	}
	return result
}

func boundedErrorText(err error) string {
	if err == nil {
		return "unknown"
	}
	text := strings.TrimSpace(err.Error())
	if len(text) > maxBoundedErrorText {
		text = text[:maxBoundedErrorText]
	}
	return text
}
