// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import "fmt"

// ActivationDependency names a prerequisite for collector Init/Check. Ready is a
// bounded read-only availability lookup: it must not acquire resources or start
// preparation. A non-nil error keeps accepted enabled jobs waiting for a named
// notification. Names must be stable, non-secret and bounded in cardinality.
// Kinds vnode and secretstore are reserved for framework dependencies.
type ActivationDependency struct {
	Kind  string
	Name  string
	Ready func() error
}

// ActivationDependencyUnavailableError identifies a declared prerequisite that
// is not ready. The framework preserves it through configuration redaction.
type ActivationDependencyUnavailableError struct {
	Kind  string
	Name  string
	Cause error
}

func (e *ActivationDependencyUnavailableError) Error() string {
	return fmt.Sprintf("activation dependency %s/%s is unavailable", e.Kind, e.Name)
}

func (e *ActivationDependencyUnavailableError) Unwrap() error { return e.Cause }
