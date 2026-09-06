// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

var lifecyclePhases = []string{"unknown", "init", "check", "collect"}
var lifecycleOutcomes = []string{"unknown", "success", "failed"}

func LifecyclePhaseName(v ddsnmp.DeviceLifecyclePhase) (string, error) {
	if int(v) >= len(lifecyclePhases) {
		return "", fmt.Errorf("unknown value %d", v)
	}
	return lifecyclePhases[v], nil
}
func ParseLifecyclePhase(v string) (ddsnmp.DeviceLifecyclePhase, error) {
	for i, n := range lifecyclePhases {
		if n == v {
			return ddsnmp.DeviceLifecyclePhase(i), nil
		}
	}
	return 0, fmt.Errorf("unknown value %q", v)
}
func LifecycleOutcomeName(v ddsnmp.DeviceLifecycleOutcome) (string, error) {
	if int(v) >= len(lifecycleOutcomes) {
		return "", fmt.Errorf("unknown value %d", v)
	}
	return lifecycleOutcomes[v], nil
}
func ParseLifecycleOutcome(v string) (ddsnmp.DeviceLifecycleOutcome, error) {
	for i, n := range lifecycleOutcomes {
		if n == v {
			return ddsnmp.DeviceLifecycleOutcome(i), nil
		}
	}
	return 0, fmt.Errorf("unknown value %q", v)
}

func NewLifecycle(cut ddsnmp.DeviceLifecycleCut) (Lifecycle, error) {
	result := Lifecycle{
		State:  "available",
		Reason: "none",
		Cut: LifecycleCut{
			Sequence:   cut.Sequence,
			CapturedAt: cut.CapturedAt,
			Entries:    make([]LifecycleEntry, 0, len(cut.Entries)),
		},
	}
	for _, entry := range cut.Entries {
		phase, err := LifecyclePhaseName(entry.LastCompleted.Phase)
		if err != nil {
			return Lifecycle{}, err
		}
		outcome, err := LifecycleOutcomeName(entry.LastCompleted.Outcome)
		if err != nil {
			return Lifecycle{}, err
		}
		result.Cut.Entries = append(
			result.Cut.Entries,
			LifecycleEntry{
				RegistrationID: uint64(entry.RegistrationID),
				Hostname:       entry.Info.Hostname,
				Port:           entry.Info.Port,
				SNMPVersion:    entry.Info.SNMPVersion,
				LastCompleted: LifecycleStatus{
					Phase:       phase,
					Outcome:     outcome,
					CompletedAt: entry.LastCompleted.CompletedAt,
				},
				TopologyReady: entry.TopologyReady,
			},
		)
	}
	return result, nil
}

const MaxRecords uint64 = 250_000
const MaxLogicalBytes uint64 = 64 << 20
const LifecycleCutLogicalBytes uint64 = 32

type LifecycleSource interface {
	LifecycleCut() ddsnmp.DeviceLifecycleCut
}

func CaptureLifecycle(source LifecycleSource, maxRecords, maxBytes uint64) (result Lifecycle) {
	result = Lifecycle{
		State:  "unavailable",
		Reason: "projection_error",
	}
	defer func() {
		if recover() != nil {
			result = Lifecycle{
				State:  "unavailable",
				Reason: "projection_panic",
			}
		}
	}()
	if source == nil {
		return result
	}
	cut := source.LifecycleCut()
	records := uint64(1 + len(cut.Entries))
	size := LifecycleCutLogicalBytes
	for _, entry := range cut.Entries {
		size += uint64(64 + len(entry.Info.Hostname) + len(entry.Info.SNMPVersion))
	}
	if records > maxRecords || size > maxBytes {
		result = Lifecycle{
			State:  "limit_exceeded",
			Reason: "global_byte_limit",
			Cut: LifecycleCut{
				Sequence:   cut.Sequence,
				CapturedAt: cut.CapturedAt,
			},
		}
		if records > maxRecords {
			result.Reason = "global_record_limit"
		}
		return result
	}
	projected, err := NewLifecycle(cut)
	if err != nil {
		return result
	}
	return projected
}
