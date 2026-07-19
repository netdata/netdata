// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestFunctionCleanupQueuePreservesFIFOAndReleasesChunks(t *testing.T) {
	const population = 3*functionCleanupChunkCapacity + 7

	var queue functionCleanupQueue
	for index := range population {
		plan := FunctionCleanupPlan{
			Ref: FunctionCleanupRef{
				Slot:       uint32(index + 1),
				Generation: 1,
			},
			Work: func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		}
		if err := queue.push(plan); err != nil {
			t.Fatal(err)
		}
	}
	if queue.count != population {
		t.Fatalf("queued plans=%d, want %d", queue.count, population)
	}
	chunks := 0
	for chunk := queue.head; chunk != nil; chunk = chunk.next {
		chunks++
	}
	const wantChunks = 4
	if chunks != wantChunks {
		t.Fatalf("queue chunks=%d, want %d", chunks, wantChunks)
	}

	firstChunk := queue.head
	for index := range population {
		plan := queue.front()
		if plan.Ref.Slot != uint32(index+1) ||
			plan.Ref.Generation != 1 {
			t.Fatalf(
				"front at %d=%+v",
				index,
				plan.Ref,
			)
		}
		queue.pop()
		if index == functionCleanupChunkCapacity-1 {
			if firstChunk.next != nil {
				t.Fatal("exhausted queue chunk remained linked")
			}
			for slot, plan := range firstChunk.plans {
				if plan.Ref.Valid() || plan.Work != nil || plan.Runner != nil {
					t.Fatalf(
						"exhausted chunk retained plan %d: %+v",
						slot,
						plan.Ref,
					)
				}
			}
		}
	}
	if queue.count != 0 || queue.head != nil || queue.tail != nil {
		t.Fatalf(
			"drained queue retained state: count=%d head=%p tail=%p",
			queue.count,
			queue.head,
			queue.tail,
		)
	}
	if plan := queue.front(); plan.Ref.Valid() ||
		plan.Work != nil || plan.Runner != nil {
		t.Fatalf("empty queue returned plan %+v", plan.Ref)
	}
}

func BenchmarkBFunctionCleanupQueuePushPop(b *testing.B) {
	const population = 4 * functionCleanupChunkCapacity
	plans := make([]FunctionCleanupPlan, population)
	for index := range plans {
		plans[index] = FunctionCleanupPlan{
			Ref: FunctionCleanupRef{
				Slot:       uint32(index + 1),
				Generation: 1,
			},
			Work: func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		var queue functionCleanupQueue
		for _, plan := range plans {
			if err := queue.push(plan); err != nil {
				b.Fatal(err)
			}
		}
		for queue.count != 0 {
			queue.pop()
		}
	}
}
