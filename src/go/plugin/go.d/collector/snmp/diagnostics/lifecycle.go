// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
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
				Profiles:       entry.Info.Profiles.Snapshot(),
				Hostname:       entry.Info.Hostname,
				Port:           entry.Info.Port,
				SNMPVersion:    entry.Info.SNMPVersion,
				LastCompleted: LifecycleStatus{
					Phase:              phase,
					Failure:            entry.LastCompleted.Failure,
					PreparationFailure: entry.LastCompleted.PreparationFailure,
					CollectionFailures: entry.LastCompleted.CollectionFailures,
					Outcome:            outcome,
					CompletedAt:        entry.LastCompleted.CompletedAt,
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
		size += LifecycleEntryLogicalBytes(entry.Info.Hostname, entry.Info.SNMPVersion)
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
	cut.Entries = slices.Clone(cut.Entries)
	for i := range cut.Entries {
		context := cut.Entries[i].Info.Profiles
		if context == nil {
			continue
		}
		contextRecords, contextBytes := context.Shape()
		if contextRecords > maxRecords-records || contextBytes > maxBytes-size {
			limited, _ := ddsnmp.RestoreProfileContext(ddsnmp.ProfileContextData{State: "limit_exceeded"}, 0, 0)
			cut.Entries[i].Info.Profiles = limited
			continue
		}
		records += contextRecords
		size += contextBytes
	}
	projected, err := NewLifecycle(cut)
	if err != nil {
		return result
	}
	return projected
}

// LifecycleEntryLogicalBytes includes fixed failure and unavailable-context slots.
func LifecycleEntryLogicalBytes(hostname, version string) uint64 {
	const preparationFailureBytes = 128
	return uint64(
		64+len(hostname)+len(version),
	) + snmputils.FailureLogicalBytes + preparationFailureBytes + ddsnmp.CollectionFailuresLogicalBytes + 32
}
