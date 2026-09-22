// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
)

const (
	maxBoundedErrorClasses = 12
	maxBoundedErrorText    = 256
)

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
	return &boundedOperationError{
		total:           a.total,
		representatives: representatives,
	}
}

func boundedErrorClass(err error) string {
	switch {
	case errors.Is(err, identity.ErrIntegrity):
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
