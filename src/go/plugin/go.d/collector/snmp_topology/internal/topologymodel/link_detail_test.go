// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLinkBGPRoutingInstance(t *testing.T) {
	assert.Empty(t, LinkBGPRoutingInstance(Link{}))
	for name, tc := range map[string]struct {
		value string
		want  string
	}{
		"absent":             {},
		"blank":              {value: " \r\n\t"},
		"default":            {value: "default", want: "default"},
		"space boundaries":   {value: " edge ", want: " edge "},
		"control boundaries": {value: "\r\nedge\t", want: "\r\nedge\t"},
	} {
		t.Run(name, func(t *testing.T) {
			link := Link{
				Detail: LinkDetail{
					BGP: &BGPAdjacencyLinkDetail{
						RoutingInstance: tc.value,
					},
				},
			}
			assert.Equal(t, tc.want, LinkBGPRoutingInstance(link))
		})
	}
}
