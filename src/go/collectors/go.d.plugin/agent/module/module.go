// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"github.com/netdata/netdata/go/go.d.plugin/logger"
)

// Module is an interface that represents a module.
type Module interface {
	// Init does initialization.
	// If it returns false, the job will be disabled.
	Init() bool

	// Check is called after Init.
	// If it returns false, the job will be disabled.
	Check() bool

	// Charts returns the chart definition.
	// Make sure not to share returned instance.
	Charts() *Charts

	// Collect collects metrics.
	Collect() map[string]int64

	// Cleanup Cleanup
	Cleanup()

	GetBase() *Base
}

// Base is a helper struct. All modules should embed this struct.
type Base struct {
	*logger.Logger
}

func (b *Base) GetBase() *Base { return b }
