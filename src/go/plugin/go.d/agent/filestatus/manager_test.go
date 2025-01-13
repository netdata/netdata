// SPDX-License-Identifier: GPL-3.0-or-later

package filestatus

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager("")
	assert.NotNil(t, mgr.store)
}

func TestManager_Run(t *testing.T) {
	type testAction struct {
		name   string
		cfg    confgroup.Config
		status string
	}
	tests := map[string]struct {
		actions  []testAction
		wantFile string
	}{
		"save": {
			actions: []testAction{
				{
					name: "save", status: "ok",
					cfg: prepareConfig("module", "module1", "name", "name1"),
				},
				{
					name: "save", status: "ok",
					cfg: prepareConfig("module", "module2", "name", "name2"),
				},
			},
			wantFile: `
{
 "module1": {
  "name1:17896517344060997937": "ok"
 },
 "module2": {
  "name2:14519194242031159283": "ok"
 }
}
`,
		},
		"remove": {
			actions: []testAction{
				{
					name: "save", status: "ok",
					cfg: prepareConfig("module", "module1", "name", "name1"),
				},
				{
					name: "save", status: "ok",
					cfg: prepareConfig("module", "module2", "name", "name2"),
				},
				{
					name: "remove",
					cfg:  prepareConfig("module", "module2", "name", "name2"),
				},
			},
			wantFile: `
{
 "module1": {
  "name1:17896517344060997937": "ok"
 }
}
`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "netdata-go-test-filestatus-run")
			require.NoError(t, err)
			defer func() { assert.NoError(t, os.RemoveAll(dir)) }()

			filename := path.Join(dir, "filestatus")

			mgr := NewManager(filename)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); mgr.Run(ctx) }()

			for _, v := range test.actions {
				switch v.name {
				case "save":
					mgr.Save(v.cfg, v.status)
				case "remove":
					mgr.Remove(v.cfg)
				}
			}

			cancel()

			timeout := time.Second * 5
			tk := time.NewTimer(timeout)
			defer tk.Stop()

			select {
			case <-done:
			case <-tk.C:
				t.Errorf("timed out after %s", timeout)
			}

			bs, err := os.ReadFile(filename)
			require.NoError(t, err)

			assert.Equal(t, strings.TrimSpace(test.wantFile), string(bs))
		})
	}
}
