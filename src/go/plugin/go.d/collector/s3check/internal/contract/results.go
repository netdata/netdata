// SPDX-License-Identifier: GPL-3.0-or-later

package contract

import "time"

func FailedProbe(reason Reason) *ProbeResult {
	return &ProbeResult{
		Status: StatusFailed,
		Reason: reason,
	}
}

func CloneProbe(value *ProbeResult) *ProbeResult {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func ObjectiveResultFor(lag, objective time.Duration) ObjectiveResult {
	status := StatusSuccess
	if lag > objective {
		status = StatusFailed
	}
	return ObjectiveResult{
		Performed: true,
		Status:    status,
		Lag:       lag,
	}
}
