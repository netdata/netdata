// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistryDetachIsHandleIdentitySafe(t *testing.T) {
	registry := NewRegistry()
	stale := registry.Attach("listener", Options{})
	current := registry.Attach("listener", Options{})

	stale.Detach()
	assert.Same(t, current, registry.jobs["listener"])

	current.Detach()
	assert.NotContains(t, registry.jobs, "listener")
}
