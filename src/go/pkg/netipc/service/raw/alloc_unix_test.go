//go:build unix

package raw

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// Typed calls on an established session, client and server in this
// process, must not allocate on either side.
func assertCallsDoNotAllocate(t *testing.T, svc string, scfg posix.ServerConfig, ccfg posix.ClientConfig) {
	t.Helper()
	skipAllocTestsUnderRace(t)
	for _, tc := range allocCallCases() {
		t.Run(tc.name, func(t *testing.T) {
			name := svc + "_" + strings.ReplaceAll(tc.name, "-", "_")
			ensureRunDir()
			cleanupAll(name)
			trackTestLock(testRunDir, name)
			ts := startTestServerUnixWithConfig(name, scfg, tc.method, tc.dispatch)
			defer ts.stop()

			client := newClient(testRunDir, name, ccfg, tc.method)
			defer client.Close()
			client.Refresh()
			if !client.Ready() {
				t.Fatal("client not ready")
			}

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

func TestCallNoAllocsBaseline(t *testing.T) {
	assertCallsDoNotAllocate(t, "go_alloc_uds", testServerConfig(), testClientConfig())
}
