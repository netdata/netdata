// SPDX-License-Identifier: GPL-3.0-or-later

package privilege

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func capabilitiesAtExec() (bool, error) {
	// Unlike auxv, status remains readable by nonroot file-capability processes.
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false, errors.New("cannot read Linux capability state")
	}
	return gainedCapabilitiesAtExec(status)
}

// Permitted capabilities beyond the inherited ambient set identify a gain even
// when effective is empty. The file effective bit is not recoverable from status;
// any effective capability conservatively counts as a gain, matching eBPF policy.
func gainedCapabilitiesAtExec(status []byte) (bool, error) {
	permitted, found, err := capabilitySet(status, "CapPrm:")
	if err != nil {
		return false, err
	}
	if !found {
		return false, errors.New("missing CapPrm capability field")
	}
	ambient, _, ambientErr := capabilitySet(status, "CapAmb:")
	// Kernels before 4.3 have no ambient set: any permitted capability is a gain.
	effective, found, effectiveErr := capabilitySet(status, "CapEff:")
	if effectiveErr == nil && !found {
		effectiveErr = errors.New("missing CapEff capability field")
	}
	return permitted&^ambient != 0 || effective != 0, errors.Join(ambientErr, effectiveErr)
}

func capabilitySet(status []byte, field string) (uint64, bool, error) {
	for line := range strings.SplitSeq(string(status), "\n") {
		value, found := strings.CutPrefix(line, field)
		if !found {
			continue
		}
		mask, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
		if err != nil {
			return 0, true, fmt.Errorf("invalid %s capability field", strings.TrimSuffix(field, ":"))
		}
		return mask, true, nil
	}
	return 0, false, nil
}
