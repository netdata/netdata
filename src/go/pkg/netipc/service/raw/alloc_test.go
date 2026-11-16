package raw

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Steady-state requests on an established session must not allocate: per
// request garbage drives GC cycles that stall the server and, through the
// peer's futex sleeps, cost far more than the allocations themselves.

const allocTestBufSize = 64 * 1024

// sessionAllocs runs one server session over requests copies of payload and
// returns the allocations of the whole session.
func sessionAllocs(t *testing.T, s *Server, code uint16, payload []byte, requests int) float64 {
	t.Helper()
	sent := 0
	served := 0
	ops := serverSessionOps{
		maxRequestPayloadBytes:  allocTestBufSize,
		maxResponsePayloadBytes: allocTestBufSize,
		receive: func([]byte) (protocol.Header, []byte, serverReceiveAction) {
			if sent == requests {
				return protocol.Header{}, nil, serverReceiveStop
			}
			sent++
			return protocol.Header{
				Kind:       protocol.KindRequest,
				Code:       code,
				ItemCount:  1,
				MessageID:  uint64(sent),         // #nosec G115 -- sent is a small positive test count.
				PayloadLen: uint32(len(payload)), // #nosec G115 -- test payloads are small.
			}, payload, serverReceiveOK
		},
		send: func(hdr *protocol.Header, _ []byte, _ *[]byte) error {
			if hdr.TransportStatus != protocol.StatusOK {
				t.Errorf("response status %d", hdr.TransportStatus)
			}
			served++
			return nil
		},
		close: func() {},
	}
	s.running.Store(true)
	allocs := testing.AllocsPerRun(5, func() {
		sent = 0
		s.handleServerSession(ops)
	})
	if served == 0 {
		t.Fatal("no request served")
	}
	return allocs
}

// assertNoPerRequestAllocs compares a 1-request session with a 2000-request
// one. Per-session buffers are the same in both, so a per-request
// allocation shows up as at least 1999 more; the margin absorbs the Go
// runtime's own occasional allocations (starting an OS thread), which are
// not per request.
func assertNoPerRequestAllocs(t *testing.T, s *Server, code uint16, payload []byte) {
	t.Helper()
	if raceEnabled {
		t.Skip("sync.Pool drops items under the race detector")
	}
	const requests = 2000
	one := sessionAllocs(t, s, code, payload, 1)
	many := sessionAllocs(t, s, code, payload, requests)
	if many-one >= requests/100 {
		t.Fatalf("allocations: %.0f for 1 request, %.0f for %d requests", one, many, requests)
	}
}

func TestServerNoPerRequestAllocsIncrement(t *testing.T) {
	s := &Server{
		expectedMethodCode: protocol.MethodIncrement,
		handler:            IncrementDispatch(func(v uint64) (uint64, bool) { return v + 1, true }),
	}
	var req [protocol.IncrementPayloadSize]byte
	protocol.IncrementEncode(41, req[:])
	assertNoPerRequestAllocs(t, s, protocol.MethodIncrement, req[:])
}

func TestServerNoPerRequestAllocsSnapshot(t *testing.T) {
	name := []byte("cgroup-1")
	path := []byte("/sys/fs/cgroup/bench/cg-1")
	s := &Server{
		expectedMethodCode: protocol.MethodCgroupsSnapshot,
		handler: SnapshotDispatch(func(_ *protocol.CgroupsRequest, b *protocol.CgroupsBuilder) bool {
			b.SetHeader(1, 7)
			for i := range uint32(16) {
				if b.Add(1000+i, 0, i%2, name, path) != nil {
					return false
				}
			}
			return true
		}, 16),
	}
	req := protocol.CgroupsRequest{LayoutVersion: 1}
	var buf [4]byte
	req.Encode(buf[:])
	assertNoPerRequestAllocs(t, s, protocol.MethodCgroupsSnapshot, buf[:])
}

func TestServerNoPerRequestAllocsCgroupsLookup(t *testing.T) {
	name := []byte("cg")
	s := &Server{
		expectedMethodCode: protocol.MethodCgroupsLookup,
		handler: CgroupsLookupDispatch(func(req *protocol.CgroupsLookupRequestView, b *protocol.CgroupsLookupBuilder) bool {
			for i := range req.ItemCount {
				if b.AddRequestItem(req, i, protocol.CgroupLookupKnown, 0, name, nil) != nil {
					return false
				}
			}
			return true
		}),
	}
	paths := [][]byte{[]byte("/a"), []byte("/b/c"), []byte("/d")}
	buf := make([]byte, 256)
	n, err := protocol.EncodeCgroupsLookupRequest(paths, buf)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	assertNoPerRequestAllocs(t, s, protocol.MethodCgroupsLookup, buf[:n])
}

func TestServerNoPerRequestAllocsAppsLookup(t *testing.T) {
	comm := []byte("proc")
	cgPath := []byte("/sys/fs/cgroup/app")
	cgName := []byte("app")
	s := &Server{
		expectedMethodCode: protocol.MethodAppsLookup,
		handler: AppsLookupDispatch(func(req *protocol.AppsLookupRequestView, b *protocol.AppsLookupBuilder) bool {
			for i := range req.ItemCount {
				pid, err := req.Item(i)
				if err != nil {
					return false
				}
				if b.Add(protocol.PidLookupKnown, protocol.AppsCgroupKnown, 0, pid, 0, 0, 1, comm, cgPath, cgName, nil) != nil {
					return false
				}
			}
			return true
		}),
	}
	buf := make([]byte, 256)
	n, err := protocol.EncodeAppsLookupRequest([]uint32{10, 20, 30}, buf)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	assertNoPerRequestAllocs(t, s, protocol.MethodAppsLookup, buf[:n])
}
