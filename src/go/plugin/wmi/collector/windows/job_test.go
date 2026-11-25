// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type wireOutput chan string

func (w wireOutput) Write(p []byte) (int, error) {
	select {
	case w <- string(p):
	default:
	}
	return len(p), nil
}

func TestCollector_JobV2Publication(t *testing.T) {
	const namedGUID = "245dea48-ae2a-4697-a22e-050e7e358afb"
	for name, vnode := range map[string]vnodes.VirtualNode{
		"generated vnode": {},
		"named vnode": {
			Name:     "override",
			Hostname: "configured.example.com",
			GUID:     namedGUID,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			c.open = func(context.Context, string, string, string, string) (queryClient, error) {
				return &fakeSession{
					results: validQueryResults(),
				}, nil
			}
			out := make(wireOutput, 16)
			job := jobruntime.NewJobV2(jobruntime.JobV2Config{
				PluginName:  "wmi",
				Name:        "fixture",
				ModuleName:  "windows",
				FullName:    "windows_fixture",
				Module:      c,
				Out:         out,
				UpdateEvery: c.UpdateEvery,
				Vnode:       vnode,
				VnodeName:   vnode.Name,
			})
			require.NoError(t, job.AutoDetectionManaged(t.Context()))
			assert.Equal(t, 0, job.AutoDetectionEvery())

			run := jobruntime.NewManagedRun(t.Context(), nil)
			done := make(chan struct{})
			go func() {
				defer close(done)
				job.StartManaged(run)
			}()
			t.Cleanup(func() {
				run.Stop(nil)
				<-done
				job.Cleanup()
			})
			<-run.StartupDone()
			require.NoError(t, run.StartupErr())

			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			for {
				select {
				case <-ticker.C:
					job.Tick(c.UpdateEvery)
				case output := <-out:
					if !strings.Contains(output, "SET 'received' = 190") {
						continue
					}
					if vnode.Name == "" {
						assert.Contains(t, output, "HOST_DEFINE '"+c.VirtualNode().GUID+"' 'windows.example.com'")
						assert.Contains(t, output, "HOST_LABEL '_vnode_type' 'wmi'")
					} else {
						assert.Contains(t, output, "HOST_DEFINE '"+namedGUID+"' 'configured.example.com'")
						assert.NotContains(t, output, c.VirtualNode().GUID)
					}
					assert.Contains(t, output, "CLABEL 'interface' 'Ethernet'")
					assert.Contains(t, output, "SET 'busy' = 0")
					assert.Contains(t, output, "SET 'used' = 6442450944")
					return
				case <-ctx.Done():
					t.Fatal("the job did not emit Windows metric data")
				}
			}
		})
	}
}
