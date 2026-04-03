// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
)

func TestJobLogSource(t *testing.T) {
	cfg := confgroup.Config{}
	cfg.SetSourceType(confgroup.TypeDiscovered)
	cfg.SetProvider("file watcher")

	assert.Equal(t, "discovered/file watcher", jobLogSource(cfg))
}
