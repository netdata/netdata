// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
)

func TestJobLogSource(t *testing.T) {
	tests := map[string]struct {
		sourceType string
		provider   string
		want       string
	}{
		"different source type and provider": {
			sourceType: confgroup.TypeDiscovered,
			provider:   "file watcher",
			want:       "discovered/file watcher",
		},
		"same source type and provider": {
			sourceType: confgroup.TypeDyncfg,
			provider:   confgroup.TypeDyncfg,
			want:       confgroup.TypeDyncfg,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := confgroup.Config{}
			cfg.SetSourceType(tc.sourceType)
			cfg.SetProvider(tc.provider)

			assert.Equal(t, tc.want, jobLogSource(cfg))
		})
	}
}
