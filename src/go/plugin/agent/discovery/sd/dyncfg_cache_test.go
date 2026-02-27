// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
)

func TestSourceTypeFromPath(t *testing.T) {
	tests := map[string]struct {
		path string
		want string
	}{
		"user path under /etc": {
			path: "/etc/netdata/sd.d/test.conf",
			want: confgroup.TypeUser,
		},
		"stock path outside /etc": {
			path: "/usr/lib/netdata/conf.d/sd.d/test.conf",
			want: confgroup.TypeStock,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, sourceTypeFromPath(tc.path))
		})
	}
}
