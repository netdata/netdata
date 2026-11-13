//go:build unix && netipc_extended

package raw

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// simpleHash: djb2 matching the C implementation
func simpleHash(s string) uint32 {
	var hash uint32 = 5381
	for _, c := range []byte(s) {
		hash = ((hash << 5) + hash) + uint32(c)
	}
	return hash
}

// largeHandler builds a snapshot with N items (realistic container names).
func largeHandler(n int) DispatchHandler {
	return SnapshotDispatch(func(request *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
		if request.LayoutVersion != 1 || request.Flags != 0 {
			return false
		}
		builder.SetHeader(1, 42)

		for i := range n {
			name := fmt.Sprintf("container-%04d", i)
			path := fmt.Sprintf("/sys/fs/cgroup/docker/%04d", i)
			hash := simpleHash(name)
			enabled := uint32(1)
			if i%5 == 0 {
				enabled = 0
			}
			if err := builder.Add(hash, 0x10, enabled,
				[]byte(name), []byte(path)); err != nil {
				return false
			}
		}
		return true
	}, uint32(n))
}

// startServerWithWorkers creates a server with explicit worker count.
func startServerWithWorkers(service string, handler DispatchHandler, workers int) *testServer {
	ensureRunDir()
	cleanupAll(service)

	s := NewServerWithWorkers(
		testRunDir,
		service,
		testServerConfig(),
		protocol.MethodCgroupsSnapshot,
		handler,
		workers,
	)
	doneCh := runServerForTest(s)
	return &testServer{server: s, doneCh: doneCh}
}
