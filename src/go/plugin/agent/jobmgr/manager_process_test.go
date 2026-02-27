// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
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

func TestRun_WaitTimeoutClearsGateAndKeepsAccepted(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})
	mgr.modules = prepareMockRegistry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx

	done := make(chan struct{})
	go func() {
		mgr.run()
		close(done)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("run did not stop after cancel")
		}
	}()

	cfg1 := prepareStockCfg("success", "wait1")
	cfg2 := prepareStockCfg("success", "wait2")

	mgr.addCh <- cfg1
	require.Eventually(t, mgr.handler.WaitingForDecision, time.Second, 10*time.Millisecond)

	secondSent := make(chan struct{})
	go func() {
		mgr.addCh <- cfg2
		close(secondSent)
	}()

	select {
	case <-secondSent:
		t.Fatal("second add was processed before wait timeout")
	case <-time.After(500 * time.Millisecond):
	}

	select {
	case <-secondSent:
	case <-time.After(7 * time.Second):
		t.Fatal("second add did not progress after wait timeout")
	}

	entry1, ok := mgr.exposed.LookupByKey(cfg1.ExposedKey())
	require.True(t, ok, "first config must stay exposed after timeout")
	assert.Equal(t, dyncfg.StatusAccepted, entry1.Status)
}
