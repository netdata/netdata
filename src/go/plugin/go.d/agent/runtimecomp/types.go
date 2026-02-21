// SPDX-License-Identifier: GPL-3.0-or-later

package runtimecomp

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

// AutogenPolicy controls unmatched-series fallback chart generation
// for runtime/internal components.
type AutogenPolicy struct {
	Enabled bool

	// TypeID is the chart-type prefix used by Netdata runtime checks
	// (`type.id` length guard). Typically this is `<plugin>.<job>`.
	TypeID string
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
	RegisterProducer(name string, tickFn func() error) error
}
