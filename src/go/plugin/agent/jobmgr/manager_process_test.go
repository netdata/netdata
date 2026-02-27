// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func TestRunProcessConfGroups_ChannelCloseDoesNotSpin(t *testing.T) {
	tests := map[string]struct {
		closeInput   bool
		cancelBefore bool
		wantExit     bool
	}{
		"closed channel exits loop": {
			closeInput: true,
			wantExit:   true,
		},
		"canceled context exits loop": {
			cancelBefore: true,
			wantExit:     true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := New(Config{PluginName: testPluginName})
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			mgr.ctx = ctx

			in := make(chan []*confgroup.Group)
			if tc.closeInput {
				close(in)
			}
			if tc.cancelBefore {
				cancel()
			}

			done := make(chan struct{})
			go func() {
				mgr.runProcessConfGroups(in)
				close(done)
			}()

			select {
			case <-done:
				assert.True(t, tc.wantExit)
			case <-time.After(200 * time.Millisecond):
				t.Fatal("runProcessConfGroups did not exit")
			}
		})
	}
}
