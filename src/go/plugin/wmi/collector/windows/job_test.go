// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
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
			synctest.Test(t, func(t *testing.T) {
				c := configuredCollector()
				renameAt := time.Now().Add(inventoryInterval)
				c.Vnode = vnode.Name
				c.newSession = func(context.Context) (querySession, error) {
					results := validQueryResults()
					hostname := "WIN-LAB"
					if !time.Now().Before(renameAt) {
						hostname = "WIN-RENAMED"
					}
					// Idle for three quarters of every interval: 25% busy.
					now := uint64(time.Now().UnixNano() / 100)
					results[queryCPU] = fixtureRows(map[string]any{"PercentProcessorTime": now / 4 * 3, "Timestamp_Sys100NS": now})
					results[queryOSInfo] = fixtureRows(
						map[string]any{
							"CSName":      hostname,
							"Caption":     "Microsoft Windows Server 2022 Standard",
							"ProductType": uint32(3),
						},
					)
					return &fakeSession{
						results: results,
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

				collect := func(clock int) string {
					t.Helper()
					synctest.Wait()
					job.Tick(clock)
					synctest.Wait()
					var output strings.Builder
					for len(out) > 0 {
						output.WriteString(<-out)
					}
					require.Contains(t, output.String(), "SET 'received' = 190")
					return output.String()
				}
				output := collect(c.UpdateEvery)
				assert.Contains(t, output, "CLABEL 'interface' 'Ethernet'")
				assert.NotContains(t, output, "SET 'busy'", "the first interval starts at the readiness check, at the same instant")
				assert.Contains(t, output, "SET 'used' = 6442450944")
				if vnode.Name != "" {
					assert.Contains(t, output, "HOST_DEFINE '"+namedGUID+"' 'configured.example.com'")
					assert.NotContains(t, output, c.VirtualNode().GUID)
					assert.NotContains(t, output, "HOST_LABEL '_os_edition'")
					return
				}
				guid := c.VirtualNode().GUID
				assert.Contains(t, output, "HOST_DEFINE '"+guid+"' 'WIN-LAB'")
				assert.Contains(t, output, "HOST_LABEL '_vnode_type' 'wmi'")
				assert.Contains(t, output, "HOST_LABEL 'address' 'windows.example.com'")
				assert.Contains(t, output, "HOST_LABEL '_os_edition' 'Standard'")

				time.Sleep(inventoryInterval)
				renamed := collect(2 * c.UpdateEvery)
				assert.Contains(t, renamed, "HOST_DEFINE '"+guid+"' 'WIN-RENAMED'")
				assert.Contains(t, renamed, "SET 'busy' = 25")
				assert.Contains(t, renamed, "HOST_LABEL 'address' 'windows.example.com'")
			})
		})
	}
}
