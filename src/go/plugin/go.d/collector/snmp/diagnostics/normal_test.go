// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/stretchr/testify/require"
)

func TestNormalSourceValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate  func(*NormalDevice)
		invalid bool
	}{
		"current and cached bindings": {},
		"missing operation":           {func(d *NormalDevice) { d.Sources = nil }, true},
		"duplicate identity":          {func(d *NormalDevice) { d.Sources = append(d.Sources, d.Sources[0]) }, true},
		"missing cache source":        {func(d *NormalDevice) { d.Latest.Caches[0].Sources[0].Operation = 99 }, true},
		"missing consumer source":     {func(d *NormalDevice) { d.Latest.Licensing.Sources = []SourceRef{{99, 1}} }, true},
		"missing discarded candidate": {func(d *NormalDevice) {
			d.Latest.Profiles[0].Acquisition.Routes[0].DiscardedSources = []ddsnmp.SourceBinding{{Operation: 99, OID: "1.2.3", Role: "primary"}}
		}, true},
		"incorrect requested OID":         {func(d *NormalDevice) { d.Latest.Profiles[0].Acquisition.Routes[0].Sources[0].OID = "1.2.4" }, true},
		"retained success is not failure": {func(d *NormalDevice) { d.LastFailure = &NormalAttempt{ID: 1} }, true},
	} {
		t.Run(name, func(t *testing.T) {
			now := time.Now().UTC()
			d := &NormalDevice{RegistrationID: 1, RuntimeID: 1, CapturedAt: now,
				Sources: []*ddsnmp.SourceOperation{{ContextID: 1, Ordinal: 1, StartedAt: now, Method: "get", RequestedOIDs: []string{"1.2.3"}}},
				Latest: &NormalAttempt{ID: 2, Phase: "collect", StartedAt: now, CompletedAt: now,
					Caches:   []NormalCache{{Sources: []SourceRef{{1, 1}}}},
					Profiles: []NormalProfile{{Acquisition: ddsnmpcollector.AcquisitionProfileReport{Routes: []ddsnmpcollector.AcquisitionRouteReport{{Sources: []ddsnmp.SourceBinding{{ContextID: 1, Operation: 1, OID: "1.2.3", Role: "primary"}}}}}}},
				},
			}
			if tc.mutate != nil {
				tc.mutate(d)
			}
			err := d.Validate()
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNormalNegativeCauseJSON(t *testing.T) {
	cause := ddsnmpcollector.AcquisitionNegativeCause{
		ContextID: 1, Operation: 2, ObservedAt: time.Unix(1, 0).UTC(), PDU: ddsnmp.SourcePDU{OID: "1.2.3"},
	}
	attempt := NormalAttempt{NegativeCauses: []ddsnmpcollector.AcquisitionNegativeCause{cause}}
	encoded, err := json.Marshal(attempt)
	require.NoError(t, err)
	var decoded struct {
		NegativeCauses []map[string]json.RawMessage `json:"negative_causes"`
	}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Len(t, decoded.NegativeCauses, 1)
	require.Len(t, decoded.NegativeCauses[0], 4)
	for name, tc := range map[string]struct{ key string }{
		"context": {"context_id"}, "operation": {"operation"}, "timestamp": {"observed_at"}, "pdu": {"pdu"},
	} {
		t.Run(name, func(t *testing.T) { require.Contains(t, decoded.NegativeCauses[0], tc.key) })
	}
	var restored NormalAttempt
	require.NoError(t, json.Unmarshal(encoded, &restored))
	require.Equal(t, attempt, restored)
}
