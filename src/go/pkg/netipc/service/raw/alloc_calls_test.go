package raw

import (
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// allocCallCase is one typed call checked for per-call allocations. Its
// handler must not allocate either, so the server side stays clean.
type allocCallCase struct {
	name     string
	method   uint16
	dispatch DispatchHandler
	call     func(*Client) error
}

// Package-level, unlike testCgroupsHandler's items, so the handler does not
// allocate per request.
var allocCgroupsItems = [...]struct {
	hash, options, enabled uint32
	name, path             []byte
}{
	{1001, 0, 1, []byte("docker-abc123"), []byte("/sys/fs/cgroup/docker/abc123")},
	{2002, 0, 1, []byte("k8s-pod-xyz"), []byte("/sys/fs/cgroup/kubepods/xyz")},
	{3003, 0, 0, []byte("systemd-user"), []byte("/sys/fs/cgroup/user.slice/user-1000")},
}

func allocCgroupsHandler(_ *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
	builder.SetHeader(1, 42)
	for i := range allocCgroupsItems {
		item := &allocCgroupsItems[i]
		if builder.Add(item.hash, item.options, item.enabled, item.name, item.path) != nil {
			return false
		}
	}
	return true
}

var errAllocCallResult = errors.New("unexpected call result")

func allocCallCases() []allocCallCase {
	var val uint64
	return []allocCallCase{
		{
			name:   "increment",
			method: protocol.MethodIncrement,
			dispatch: IncrementDispatch(func(v uint64) (uint64, bool) {
				return v + 1, true
			}),
			call: func(c *Client) error {
				got, err := c.CallIncrement(val)
				if err != nil {
					return err
				}
				if got != val+1 {
					return errAllocCallResult
				}
				val = got
				return nil
			},
		},
		{
			name:     "snapshot",
			method:   protocol.MethodCgroupsSnapshot,
			dispatch: SnapshotDispatch(allocCgroupsHandler, uint32(len(allocCgroupsItems))),
			call: func(c *Client) error {
				view, err := c.CallSnapshot()
				if err != nil {
					return err
				}
				if view.ItemCount != uint32(len(allocCgroupsItems)) || view.Generation != 42 {
					return errAllocCallResult
				}
				return nil
			},
		},
		{
			name:   "string-reverse",
			method: protocol.MethodStringReverse,
			// Comparing a converted []byte with a constant does not allocate.
			dispatch: StringReverseDispatch(func(s []byte) (string, bool) {
				if string(s) != "hello" {
					return "", false
				}
				return "olleh", true
			}),
			call: func(c *Client) error {
				view, err := c.CallStringReverse("hello")
				if err != nil {
					return err
				}
				if string(view.Str) != "olleh" {
					return errAllocCallResult
				}
				return nil
			},
		},
	}
}

func allocCallError(name string, err error) error {
	return fmt.Errorf("%s call: %w", name, err)
}

// allocCallRuns calls per case; the runtime's own occasional allocations
// (starting an OS thread) stay below allocCallRuns/100 in total.
const allocCallRuns = 2000

// callAllocs returns the heap allocations, client and in-process server
// together, of allocCallRuns calls after one warm-up call. Unlike
// testing.AllocsPerRun it does not round the per-call average down, so an
// allocation made on only some calls still counts.
func callAllocs(f func()) uint64 {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	f()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range allocCallRuns {
		f()
	}
	runtime.ReadMemStats(&after)
	return after.Mallocs - before.Mallocs
}

func skipAllocTestsUnderRace(t *testing.T) {
	t.Helper()
	if raceEnabled {
		t.Skip("sync.Pool drops items under the race detector")
	}
}
