// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/stretchr/testify/require"
)

func TestDynCfgResolveRequestReportsArgumentErrors(t *testing.T) {
	tests := map[string]struct {
		args        []string
		wantFailure dynCfgFailure
	}{
		"empty Add name": {
			args:        []string{"go.d:collector:module", "add", ""},
			wantFailure: newDynCfgFailure(failureBadRequest, "invalid or missing job name."),
		},
		"missing Add name argument": {
			args:        []string{"go.d:collector:module", "add"},
			wantFailure: newDynCfgFailure(failureBadRequest, "missing required arguments: need 3, got 2"),
		},
		"invalid non-empty Add name": {
			args:        []string{"go.d:collector:module", "add", "job.name"},
			wantFailure: newDynCfgFailure(failureBadRequest, "Unacceptable job name 'job.name': contains '.'."),
		},
		"Add name with a colon is not rewritten": {
			args:        []string{"go.d:collector:module", "add", "a:b"},
			wantFailure: newDynCfgFailure(failureBadRequest, "Unacceptable job name 'a:b': contains ':'."),
		},
		"Add name with a space is not rewritten": {
			args:        []string{"go.d:collector:module", "add", "a b"},
			wantFailure: newDynCfgFailure(failureBadRequest, "Unacceptable job name 'a b': contains spaces."),
		},
		"Test name with a colon is not rewritten": {
			args:        []string{"go.d:collector:module", "test", "a:b"},
			wantFailure: newDynCfgFailure(failureBadRequest, "Unacceptable job name 'a:b': contains ':'."),
		},
		"non-Add command missing ID name": {
			args:        []string{"go.d:collector:module", "remove"},
			wantFailure: newDynCfgFailure(failureBadRequest, "invalid config ID format."),
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

func TestDynCfgRequestSourceUsesQuotedProtocolGrammar(t *testing.T) {
	tests := map[string]bool{
		"safe":          true,
		"user=test":     true,
		`trailing\`:     false,
		`embedded\path`: true,
		"single'quote":  false,
		"line\nbreak":   false,
	}
	for value, want := range tests {
		t.Run(value, func(t *testing.T) {
			require.Equal(t, want, netdataapi.ValidSingleQuotedProtocolField(value))
		})
	}
}
