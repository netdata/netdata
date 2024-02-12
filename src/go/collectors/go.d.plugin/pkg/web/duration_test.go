// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestDuration_UnmarshalYAML(t *testing.T) {
	var d Duration
	values := [][]byte{
		[]byte("100ms"),   // duration
		[]byte("3s300ms"), // duration
		[]byte("3"),       // int
		[]byte("3.3"),     // float
	}

	for _, v := range values {
		assert.NoError(t, yaml.Unmarshal(v, &d))
	}
}
