// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/stretchr/testify/require"
)

func TestAcquisitionEvidenceValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		contexts []AcquisitionContextEvidence
		want     string
	}{
		"missing main":  {want: "main acquisition context"},
		"repeated main": {contexts: []AcquisitionContextEvidence{{Ordinal: 0, Collection: AcquisitionPhaseEvidence{Outcome: AcquisitionPhaseSuccess}}, {Ordinal: 0}}, want: "context order"},
		"valid ordered evidence": {
			contexts: []AcquisitionContextEvidence{
				{
					Ordinal:    0,
					Collection: AcquisitionPhaseEvidence{Outcome: AcquisitionPhaseSuccess},
					Profiles: []AcquisitionProfileEvidence{
						{Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0}},
						{Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 2}},
					},
				},
				{
					Ordinal:    2,
					Collection: AcquisitionPhaseEvidence{Outcome: AcquisitionPhaseSuccess},
					Profiles: []AcquisitionProfileEvidence{
						{Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0}},
					},
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			evidence := &AcquisitionAttemptEvidence{CollectedAt: time.Unix(1, 0), FreshFor: time.Minute, CollectionContexts: tc.contexts}
			err := validateAcquisitionEvidence(evidence)
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
