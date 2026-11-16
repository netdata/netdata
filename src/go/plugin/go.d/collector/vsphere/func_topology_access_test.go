// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopologyFunctionConfigRequiresPaidPlan(t *testing.T) {
	// Viewing topology maps requires a paid plan (access 0x0017).
	cfg := vsphereTopologyFunctionConfig()
	assert.True(t, cfg.RequireCloud)
	assert.True(t, cfg.RequireCommercial)
}
