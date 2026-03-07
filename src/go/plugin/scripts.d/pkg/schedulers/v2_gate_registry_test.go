// SPDX-License-Identifier: GPL-3.0-or-later

package schedulers

import (
	"fmt"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
)

func TestV2Gate_G4_SchedulerRegistry(t *testing.T) {
	t.Run("concurrent attach/detach", func(t *testing.T) {
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
		var wg sync.WaitGroup
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
						t.Errorf("attach failed: %v", err)
						return
					}
					reg.Detach(handle)
				}
			}()
		}
		wg.Wait()

		if err := reg.Remove("gate-concurrent"); err != nil {
			t.Fatalf("remove failed: %v", err)
		}
	})

	t.Run("incompatible predicate", TestRegistryEnsureRejectsIncompatibleRuntimeFields)
	t.Run("failure keeps old host", TestRegistryEnsureFailureKeepsOldHostAndJobs)
	t.Run("default remove behavior", TestRegistryRemoveDefaultResetsBuiltinDefinition)
	t.Run("stale handle behavior", TestRegistryDetachIgnoresStaleGenerationHandle)
}
