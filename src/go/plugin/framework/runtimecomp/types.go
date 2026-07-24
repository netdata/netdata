// SPDX-License-Identifier: GPL-3.0-or-later

package runtimecomp

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

// AutogenPolicy controls unmatched-series fallback chart generation
// for runtime/internal components.
type AutogenPolicy struct {
	Enabled bool

	// Exclude suppresses autogen charts for matching unmatched metric families.
	Exclude []string

	// MaxTypeIDLen is the max allowed full `type.id` length.
	// Zero means default (1200).
	MaxTypeIDLen int
	// ExpireAfterSuccessCycles controls autogen chart/dimension expiry on
	// successful collection cycles where the series is not seen.
	// Zero disables expiry.
	ExpireAfterSuccessCycles uint64
}

// ComponentConfig declares one runtime/internal metrics component.
type ComponentConfig struct {
	Name string

	Store        metrix.RuntimeStore
	TemplateYAML []byte
	UpdateEvery  int
	Autogen      AutogenPolicy

	TypeID    string
	Plugin    string
	Module    string
	JobName   string
	JobLabels map[string]string
}

// Service is the runtime component registration seam used by components.
type Service interface {
	RegisterComponent(cfg ComponentConfig) error
	UnregisterComponent(name string)

	// QuarantineComponent removes a component and its emission state WITHOUT
	// removal/obsoletion output, returning only after any in-progress emission
	// tick (including its post-write state finalizers) has completed - after
	// it returns, no output for the component can reach the wire. Unlike
	// UnregisterComponent, which stops future snapshots but lets an
	// in-progress tick finish and later emits removal-obsolete output.
	QuarantineComponent(name string)
	// FinalizeComponent synchronously emits the component's current Store
	// state, then removes its registration and emission state. It returns only
	// after no output from that component can reach the wire.
	FinalizeComponent(name string)
	RegisterProducer(name string, tickFn func() error) error
	UnregisterProducer(name string)
}
