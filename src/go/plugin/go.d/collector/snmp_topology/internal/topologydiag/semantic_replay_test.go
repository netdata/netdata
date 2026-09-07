// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAcquisitionEvidenceValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		contexts []AcquisitionContextEvidence
		want     string
	}{
		"missing main":  {want: "main acquisition context"},
		"repeated main": {contexts: []AcquisitionContextEvidence{{Ordinal: 0, Collection: AcquisitionPhaseEvidence{Outcome: AcquisitionPhaseSuccess}}, {Ordinal: 0}}, want: "context order"},
	} {
		t.Run(name, func(t *testing.T) {
			evidence := &AcquisitionAttemptEvidence{CollectedAt: time.Unix(1, 0), FreshFor: time.Minute, CollectionContexts: tc.contexts}
			require.ErrorContains(t, validateAcquisitionEvidence(evidence), tc.want)
		})
	}
}
