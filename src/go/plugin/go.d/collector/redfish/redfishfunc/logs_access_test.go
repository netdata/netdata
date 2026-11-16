// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogsMethodRequiresSignIn(t *testing.T) {
	// Log views require a signed-in Space member (access 0x0013) on every plan.
	cfg := logsMethod(1)
	assert.True(t, cfg.RequireCloud)
	assert.False(t, cfg.RequireCommercial)
}
