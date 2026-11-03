// SPDX-License-Identifier: GPL-3.0-or-later

package functions_test

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type samplingFunctionSource struct {
	metadataFunctionSource
	sites []query.Site
}

func (s samplingFunctionSource) Sites(context.Context) ([]query.Site, error) { return s.sites, nil }

func TestHistoryHelpExplainsBiasOnlyForSelectedProblemSampling(t *testing.T) {
	handler := rumfunctions.New(samplingFunctionSource{
		sites: []query.Site{
			{
				Name:  "full",
				Label: "Full",
				Sampling: query.Sampling{
					MeasureRate:     1,
					InvestigateRate: 1,
					KeepErrors:      true,
					KeepPoorVitals:  true,
				},
			},
			{Name: "baseline", Label: "Baseline", Sampling: query.Sampling{
				MeasureRate:     0.25,
				InvestigateRate: 0.1,
			}},
			{Name: "none", Label: "None", Sampling: query.Sampling{
				MeasureRate:     1,
				InvestigateRate: 0,
			}},
			{
				Name:  "errors",
				Label: "Errors",
				Sampling: query.Sampling{
					MeasureRate:     1,
					InvestigateRate: 0.5,
					KeepErrors:      true,
				},
			},
			{
				Name:  "vitals",
				Label: "Vitals",
				Sampling: query.Sampling{
					MeasureRate:     1,
					InvestigateRate: 0,
					KeepPoorVitals:  true,
				},
			},
		},
	})
	for _, method := range []string{"rum-sessions", "rum-errors"} {
		for _, tc := range []struct {
			site   string
			biased bool
		}{
			{"", true}, {"full", false}, {"baseline", false}, {"none", false}, {"errors", true}, {"vitals", true},
		} {
			t.Run(method+"/"+tc.site, func(t *testing.T) {
				var args []string
				if tc.site != "" {
					args = []string{"site:" + tc.site}
				}
				for _, info := range []bool{false, true} {
					response := handler.HandleRaw(
						context.Background(),
						funcapi.RawMethodRequest{
							Method: method,
							Args:   args,
							Info:   info,
						},
					)
					help := response.Help
					if !info {
						require.NotNil(t, response.RawResponse)
						help = response.RawResponse["help"].(string)
					}
					assert.Contains(t, help, "Current sampling policy:")
					assert.Contains(t, help, "earlier policies")
					assert.Contains(t, help, "do not estimate all visitors or errors")
					if tc.biased {
						assert.Contains(t, help, "Problem overrides bias retained detail toward failures")
					} else {
						assert.NotContains(t, help, "Problem overrides bias retained detail toward failures")
					}
				}
			})
		}
	}
}
