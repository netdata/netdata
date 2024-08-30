// SPDX-License-Identifier: GPL-3.0-or-later

package confgroup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistry_Register(t *testing.T) {
	name := "module"
	defaults := Default{
		MinUpdateEvery:     1,
		UpdateEvery:        1,
		AutoDetectionRetry: 1,
		Priority:           1,
	}
	expected := Registry{
		name: defaults,
	}

	actual := Registry{}
	actual.Register(name, defaults)

	assert.Equal(t, expected, actual)
}

func TestRegistry_Lookup(t *testing.T) {
	name := "module"
	expected := Default{
		MinUpdateEvery:     1,
		UpdateEvery:        1,
		AutoDetectionRetry: 1,
		Priority:           1,
	}
	reg := Registry{}
	reg.Register(name, expected)

	actual, ok := reg.Lookup("module")

	assert.True(t, ok)
	assert.Equal(t, expected, actual)
}
