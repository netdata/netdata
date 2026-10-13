// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
)

func TestDecodeErrorRealtimeSurvivesCachedHostIdentityFailure(t *testing.T) {
	service := failingHostIdentity{err: errors.New("identity unavailable")}
	job := &Job{deps: Dependencies{HostIdentity: service}}

	before := time.Now().UnixMicro()
	entry := newDecodeErrorEntry("test", &receiver.DecodeFailure{
		Kind: "decode_failed",
		Err:  errors.New("bad packet"),
	}, 0, job.monotonicUsecWith(nil))
	after := time.Now().UnixMicro()

	if entry.ReceivedMonotonicUsec != 0 {
		t.Fatalf("monotonic timestamp = %d, want 0", entry.ReceivedMonotonicUsec)
	}
	if entry.ReceivedRealtimeUsec < before || entry.ReceivedRealtimeUsec > after {
		t.Fatalf("realtime timestamp = %d, want between %d and %d", entry.ReceivedRealtimeUsec, before, after)
	}
}

type failingHostIdentity struct{ err error }

func (f failingHostIdentity) FreshJournal() (hostidentity.Provider, error)   { return nil, f.err }
func (f failingHostIdentity) CachedFallback() (hostidentity.Provider, error) { return nil, f.err }
