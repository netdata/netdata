// SPDX-License-Identifier: GPL-3.0-or-later

package ipmifunc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSELMethodRequiresSignIn(t *testing.T) {
	// Log views require a signed-in Space member (access 0x0013) on every plan.
	cfg := selMethod(1)
	assert.True(t, cfg.RequireCloud)
	assert.False(t, cfg.RequireCommercial)
}
