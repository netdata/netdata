// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	assert.NotNil(t, New("testdata"))
	assert.NotNil(t, New("not_exist"))
}

func TestVnodes_Lookup(t *testing.T) {
	req := New("testdata")

	_, ok := req.Lookup("first")
	assert.True(t, ok)

	_, ok = req.Lookup("second")
	assert.True(t, ok)

	_, ok = req.Lookup("third")
	assert.False(t, ok)
}
