// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Measures admission/turn/release with real waiting callers, excluding browser
// processes. It is a local control-path trend, not a production capacity claim.
func BenchmarkAdmissionReadyJobs(b *testing.B) {
	for _, population := range []int{1, 100, 1000} {
		b.Run(fmt.Sprint(population), func(b *testing.B) {
			engine, err := New(nil, nil, nil, 1)
			if err != nil {
				b.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			holder, err := engine.acquire(ctx, executionKey{jobID: "holder"})
			if err != nil {
				b.Fatal(err)
			}
			granted := make(chan *admissionJob)
			var workers sync.WaitGroup
			workers.Add(population)
			for i := 0; i < population; i++ {
				key := executionKey{jobID: fmt.Sprintf("job-%d", i)}
				go func() {
					defer workers.Done()
					for {
						job, err := engine.acquire(ctx, key)
						if err != nil {
							return
						}
						select {
						case granted <- job:
						case <-ctx.Done():
							engine.release(job)
							return
						}
					}
				}()
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				engine.mu.Lock()
				ready := engine.ready.Len()
				engine.mu.Unlock()
				if ready == population {
					break
				}
				if time.Now().After(deadline) {
					cancel()
					engine.release(holder)
					workers.Wait()
					b.Fatal("live callers did not populate ready queue")
				}
				time.Sleep(time.Millisecond)
			}
			b.ReportAllocs()
			b.ResetTimer()
			engine.release(holder)
			for i := 0; i < b.N; i++ {
				engine.release(<-granted)
			}
			b.StopTimer()
			cancel()
			workers.Wait()
		})
	}
}
