// SPDX-License-Identifier: GPL-3.0-or-later

package snmptrapsfunc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogsFunctionConfigRequiresSignIn(t *testing.T) {
	// SNMP trap logs follow system logs: a signed-in Space member (access 0x0013) on every plan.
	cfg := LogsFunctionConfig(nil)
	assert.True(t, cfg.RequireCloud)
	assert.False(t, cfg.RequireCommercial)
}
