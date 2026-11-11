// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionSelectorMetadataSchema(t *testing.T) {
	schema := functionInfoSchema(t, "required_param")
	for name, test := range map[string]struct {
		param string
		valid bool
	}{
		"opaque option ID": {
			param: `{"id":"__job","name":"Job","type":"select","options":[{"id":"beta rack,1","name":"Beta"}]}`,
			valid: true,
		},
		"accepted alternate forms": {
			param: `{"id":"protocols","name":"Protocols","type":"multiselect","value_separator":",",
				"options":[{"id":"ipv4_tcp","name":"IPv4 TCP","aliases":["ipv4-tcp"]}]}`,
			valid: true,
		},
		"existing free-text selector": {
			param: `{"id":"labels","name":"Labels","type":"text"}`,
			valid: true,
		},
		"select still requires options": {
			param: `{"id":"jobs","name":"Jobs","type":"select"}`,
		},
		"unknown selector type": {
			param: `{"id":"jobs","name":"Jobs","type":"unknown","options":[]}`,
		},
		"aliases must be an array": {
			param: `{"id":"jobs","name":"Jobs","type":"select","options":[{"id":"a","name":"A","aliases":"alias"}]}`,
		},
		"aliases must be nonempty strings": {
			param: `{"id":"jobs","name":"Jobs","type":"select","options":[{"id":"a","name":"A","aliases":[""]}]}`,
		},
		"separator must be a string": {
			param: `{"id":"jobs","name":"Jobs","type":"multiselect","options":[],"value_separator":1}`,
		},
		"separator must not be empty": {
			param: `{"id":"jobs","name":"Jobs","type":"multiselect","options":[],"value_separator":""}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var param any
			require.NoError(t, json.Unmarshal([]byte(test.param), &param))
			err := schema.Validate(param)
			if test.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
