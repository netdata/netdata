// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDynCfgResolveRequestReportsArgumentErrors(t *testing.T) {
	tests := map[string]struct {
		args        []string
		wantFailure dynCfgFailure
	}{
		"empty Add name": {
			args:        []string{"go.d:collector:module", "add", ""},
			wantFailure: newDynCfgFailure(400, "invalid or missing job name."),
		},
		"missing Add name argument": {
			args:        []string{"go.d:collector:module", "add"},
			wantFailure: newDynCfgFailure(400, "missing required arguments: need 3, got 2"),
		},
		"invalid non-empty Add name": {
			args:        []string{"go.d:collector:module", "add", "job.name"},
			wantFailure: newDynCfgFailure(400, "Unacceptable job name 'job.name': contains '.'."),
		},
		"non-Add command missing ID name": {
			args:        []string{"go.d:collector:module", "remove"},
			wantFailure: newDynCfgFailure(400, "invalid config ID format."),
		},
	}

	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, failure := controller.resolveRequest(DynCfgJobRequest{
				Args: test.args,
			})
			require.Equal(t, test.wantFailure, failure)
		})
	}
}
