// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	assert.NotNil(t, Load("testdata"))
	assert.NotNil(t, Load("not_exist"))
}
