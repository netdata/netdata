//go:build windows

package raw

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

// Typed calls on an established session, client and server in this
// process, must not allocate on either side.
func assertWinCallsDoNotAllocate(t *testing.T, svc string, scfg windows.ServerConfig, ccfg windows.ClientConfig) {
	t.Helper()
	skipAllocTestsUnderRace(t)
	for _, tc := range allocCallCases() {
		t.Run(tc.name, func(t *testing.T) {
			name := svc + "_" + strings.ReplaceAll(tc.name, "-", "_")
			ts := startTestServerWinWithConfig(name, scfg, tc.method, tc.dispatch)
			defer ts.stop()

			client := newClient(winTestRunDir, name, ccfg, tc.method)
			defer client.Close()
			waitWinClientReady(t, client)

			var callErr error
			allocs := callAllocs(func() {
				if err := tc.call(client); err != nil && callErr == nil {
					callErr = allocCallError(tc.name, err)
				}
			})
			if callErr != nil {
				t.Fatal(callErr)
			}
			if allocs >= allocCallRuns/100 {
				t.Fatalf("%d allocations in %d calls", allocs, allocCallRuns)
			}
		})
	}
}

func TestWinCallNoAllocsBaseline(t *testing.T) {
	assertWinCallsDoNotAllocate(t, "go_win_alloc_np", testWinServerConfig(), testWinClientConfig())
}

func TestWinCallNoAllocsShm(t *testing.T) {
	assertWinCallsDoNotAllocate(t, "go_win_alloc_shm", testWinShmServerConfig(), testWinShmClientConfig())
}
