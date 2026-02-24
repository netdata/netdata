// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFileStatusContains_DoesNotDeadlock(t *testing.T) {
	tests := map[string]struct {
		statuses []string
		want     bool
	}{
		"contains returns true for present status": {
			statuses: []string{"running"},
			want:     true,
		},
		"contains returns false for missing status": {
			statuses: []string{"failed"},
			want:     false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := newFileStatus()
			cfg := prepareUserCfg("success", "job")
			s.add(cfg, "running")

			done := make(chan struct{})
			var got bool

			go func() {
				got = s.contains(cfg, tc.statuses...)
				close(done)
			}()

			select {
			case <-done:
				assert.Equal(t, tc.want, got)
			case <-time.After(200 * time.Millisecond):
				t.Fatal("fileStatus.contains deadlocked")
			}
		})
	}
}
