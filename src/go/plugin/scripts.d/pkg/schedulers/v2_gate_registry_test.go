// SPDX-License-Identifier: GPL-3.0-or-later

package schedulers

import (
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
)

func TestV2Gate_G4_SchedulerRegistry(t *testing.T) {
	t.Run("concurrent attach/detach with compatible ensure", func(t *testing.T) {
		reg := newRegistryWithFactory(&fakeHostFactory{})
		def := Definition{
			Name:           "gate-concurrent",
			Workers:        4,
			QueueSize:      16,
			LoggingEnabled: true,
			Logging:        defaultDefinition().Logging,
		}
		if err := reg.Ensure(def, nil); err != nil {
			t.Fatalf("ensure failed: %v", err)
		}

		const workers = 12
		const loops = 30
		const ensureLoops = 60

		var wg sync.WaitGroup
		errCh := make(chan error, workers+1)

		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < ensureLoops; i++ {
				compatible := def
				compatible.Labels = map[string]string{"generation": strconv.Itoa(i)}
				if err := reg.Ensure(compatible, nil); err != nil {
					errCh <- fmt.Errorf("ensure failed: %w", err)
					return
				}
			}
		}()

		for i := 0; i < workers; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < loops; j++ {
					handle, err := reg.Attach("gate-concurrent", runtime.JobRegistration{
						Spec: testRegJobSpec(fmt.Sprintf("job-%d-%d", i, j)),
					}, nil)
					if err != nil {
						errCh <- fmt.Errorf("attach failed: %w", err)
						return
					}
					reg.Detach(handle)
				}
			}()
		}
		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				t.Fatal(err)
			}
		}

		snapshot, ok := reg.Snapshot("gate-concurrent")
		if ok && len(snapshot.Jobs) > 0 {
			reg.mu.RLock()
			entry := reg.entries["gate-concurrent"]
			reg.mu.RUnlock()
			if entry == nil {
				t.Fatalf("expected scheduler entry for cleanup")
			}
			for _, job := range snapshot.Jobs {
				reg.Detach(&SchedulerJobHandle{
					scheduler:  "gate-concurrent",
					jobID:      job.JobID,
					generation: entry.generation,
				})
			}
		}

		if err := reg.Remove("gate-concurrent"); err != nil {
			t.Fatalf("remove failed: %v", err)
		}
		if _, ok := reg.Snapshot("gate-concurrent"); ok {
			t.Fatalf("expected snapshot to be absent after remove")
		}
		if _, ok := reg.Get("gate-concurrent"); ok {
			t.Fatalf("expected definition to be absent after remove")
		}
		for _, def := range reg.All() {
			if def.Name == "gate-concurrent" {
				t.Fatalf("found orphan scheduler definition after remove")
			}
		}
	})

	t.Run("incompatible predicate", TestRegistryEnsureRejectsIncompatibleRuntimeFields)
	t.Run("failure keeps old host", TestRegistryEnsureFailureKeepsOldHostAndJobs)
	t.Run("default remove behavior", TestRegistryRemoveDefaultResetsBuiltinDefinition)
	t.Run("stale handle behavior", TestRegistryDetachIgnoresStaleGenerationHandle)
}
