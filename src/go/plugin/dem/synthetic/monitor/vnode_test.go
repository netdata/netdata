// SPDX-License-Identifier: GPL-3.0-or-later
package monitor

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
)

func TestVnodeChangesFenceObservationsWithoutChangingQueuedIdentity(t *testing.T) {
	const a = "11111111-1111-4111-8111-111111111111"
	const b = "22222222-2222-4222-8222-222222222222"
	for _, admitted := range []bool{false, true} {
		for name, changes := range map[string][]string{
			"different": {b}, "aba": {b, a}, "unassigned": {""}, "same": {a},
		} {
			state := "queued/"
			if admitted {
				state = "admitted/"
			}
			t.Run(state+name, func(t *testing.T) {
				req := request()
				req.VnodeGUID = a
				h := newHarness(t, req)
				call := receive(t, h.executor.calls)
				queued := h.Now()
				if admitted {
					call.admission <- queued
					receive(t, call.schedule)
				}
				for _, guid := range changes {
					h.session.SetVnodeGUID(guid)
				}
				assert.Equal(t, a, call.request.VnodeGUID, "submitted request remains immutable")
				if !admitted {
					h.advance(t, queued.Add(time.Minute))
					h.wait(t, func(s Snapshot) bool { return s.Coalesced > 0 })
					call.admission <- h.Now()
					schedule := receive(t, call.schedule)
					assert.Equal(t, queued.UnixMicro(), schedule.QueuedUS, "placement does not requeue")
				}
				call.finish <- synthetic.Execution{Drained: true, Run: synthetic.Run{
					ID: "old", VnodeGUID: call.request.VnodeGUID, Outcome: synthetic.Success,
					CompletedUS: h.Now().UnixMicro(),
				}}
				snapshot := h.wait(t, func(s Snapshot) bool { return s.Outcomes.Success == 1 })
				assert.Equal(t, uint64(1), snapshot.Admitted)
				assert.Equal(t, name == "same", snapshot.HasObservation)
				h.advance(t, h.Now().Add(time.Minute))
				next := receive(t, h.executor.calls)
				assert.Equal(t, changes[len(changes)-1], next.request.VnodeGUID)
				next.admission <- h.Now()
				receive(t, next.schedule)
				next.finish <- synthetic.Execution{Drained: true, Run: synthetic.Run{
					ID: "new", VnodeGUID: next.request.VnodeGUID, Outcome: synthetic.Failed,
					CompletedUS: h.Now().UnixMicro(),
				}}
				snapshot = h.wait(t, func(s Snapshot) bool { return s.Outcomes.Failed == 1 })
				assert.True(t, snapshot.HasObservation)
				assert.Equal(t, "new", snapshot.Latest.ID)
				assert.Equal(t, uint64(1), snapshot.Outcomes.Success)
				h.session.SetVnodeGUID(next.request.VnodeGUID)
				assert.Equal(t, snapshot, h.session.Snapshot(h.Now()), "same GUID preserves the observation")
				h.stop(t, nil)
			})
		}
	}
}

func TestObsoleteVnodeResultStillRequiresVerifiedCompletion(t *testing.T) {
	for name, execution := range map[string]synthetic.Execution{
		"undrained":          {},
		"missing completion": {Drained: true},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, request())
			call := receive(t, h.executor.calls)
			call.admission <- h.Now()
			receive(t, call.schedule)
			h.session.SetVnodeGUID("11111111-1111-4111-8111-111111111111")
			call.finish <- execution
			assert.Error(t, receive(t, h.done))
			assert.False(t, h.session.Snapshot(h.Now()).HasObservation)
		})
	}
}
