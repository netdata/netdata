// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestDiscoveryShutdownCancelsReconcilerBeforeManager(t *testing.T) {
	identity := lifecycle.ResourceIdentity{
		ID:         discoveryResourceID,
		Generation: 1,
	}
	managerRef := lifecycle.InheritedTaskRef{
		Slot:       1,
		Generation: 1,
	}
	reconRef := lifecycle.InheritedTaskRef{
		Slot:       2,
		Generation: 1,
	}
	reconcilerCancelled := false
	err := cancelDiscoveryTasks(
		func(
			ref lifecycle.InheritedTaskRef,
			gotIdentity lifecycle.ResourceIdentity,
		) error {
			if gotIdentity != identity {
				return errors.New("identity differs")
			}
			switch ref {
			case reconRef:
				reconcilerCancelled = true
			case managerRef:
				if !reconcilerCancelled {
					return errors.New(
						"manager exited before reconciler cancellation",
					)
				}
			default:
				return errors.New("unknown inherited task")
			}
			return nil
		},
		identity,
		managerRef,
		reconRef,
	)
	if err != nil {
		t.Fatal(err)
	}
}
