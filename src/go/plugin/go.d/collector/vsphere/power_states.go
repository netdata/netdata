// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25/types"
)

var (
	defaultHostPowerStates = []string{string(types.HostSystemPowerStatePoweredOn)}
	defaultVMPowerStates   = []string{string(types.VirtualMachinePowerStatePoweredOn)}

	validHostPowerStates = []string{
		string(types.HostSystemPowerStatePoweredOn),
		string(types.HostSystemPowerStatePoweredOff),
		string(types.HostSystemPowerStateStandBy),
		string(types.HostSystemPowerStateUnknown),
	}
	validVMPowerStates = []string{
		string(types.VirtualMachinePowerStatePoweredOn),
		string(types.VirtualMachinePowerStatePoweredOff),
		string(types.VirtualMachinePowerStateSuspended),
	}
)

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func validatePowerStates(name string, values, allowed []string) error {
	allowedSet := make(map[string]bool, len(allowed))
	for _, state := range allowed {
		allowedSet[state] = true
	}

	seen := make(map[string]bool, len(values))
	for _, state := range values {
		if !allowedSet[state] {
			return fmt.Errorf("%s has invalid power state %q (valid: %s)", name, state, strings.Join(allowed, ", "))
		}
		if seen[state] {
			return fmt.Errorf("%s has duplicate power state %q", name, state)
		}
		seen[state] = true
	}

	return nil
}
