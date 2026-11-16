// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopologyFunctionConfigRequiresPaidPlan(t *testing.T) {
	// Viewing topology maps requires a paid plan (access 0x0017).
	cfg := topologyFunctionConfig(1)
	assert.True(t, cfg.RequireCloud)
	assert.True(t, cfg.RequireCommercial)
}
