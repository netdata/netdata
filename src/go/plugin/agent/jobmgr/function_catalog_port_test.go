// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestFunctionCatalogDecisionValidate(t *testing.T) {
	validPlan := WorkPlan{
		Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			return lifecycle.NewControlResult(lifecycle.ControlInternal)
		}),
	}
	validLease := FunctionInvocationRef{Slot: 1, Generation: 1}
	tests := map[string]struct {
		decision FunctionCatalogDecision
		wantErr  bool
	}{
		"resolved": {
			decision: FunctionCatalogDecision{Plan: validPlan, Lease: validLease},
		},
		"resource scoped": {
			decision: FunctionCatalogDecision{
				ResourceID: "resource", Plan: validPlan, Lease: validLease,
			},
		},
		"rejected": {
			decision: FunctionCatalogDecision{Rejected: lifecycle.ControlNotFound},
		},
		"resource identity exceeds bound": {
			decision: FunctionCatalogDecision{
				ResourceID: strings.Repeat("r", maximumRequestMetadataBytes+1),
				Plan:       validPlan,
				Lease:      validLease,
			},
			wantErr: true,
		},
		"transaction resource differs": {
			decision: FunctionCatalogDecision{
				ResourceID: "resource",
				Plan: WorkPlan{
					Transaction: &ResourceTransactionPlan{ID: "other"},
				},
				Lease: validLease,
			},
			wantErr: true,
		},
		"resolved without lease": {
			decision: FunctionCatalogDecision{Plan: validPlan},
			wantErr:  true,
		},
		"resolved internal work": {
			decision: FunctionCatalogDecision{
				Lease: validLease,
				Plan: WorkPlan{
					NoResponse: true,
					Work: func(context.Context) (lifecycle.TaskOutcome, error) {
						return lifecycle.NoValueOutcome(), nil
					},
				},
			},
			wantErr: true,
		},
		"rejection with lease": {
			decision: FunctionCatalogDecision{
				Lease: validLease, Rejected: lifecycle.ControlUnavailable,
			},
			wantErr: true,
		},
		"rejection with resource": {
			decision: FunctionCatalogDecision{
				ResourceID: "resource", Rejected: lifecycle.ControlUnavailable,
			},
			wantErr: true,
		},
		"rejection with plan": {
			decision: FunctionCatalogDecision{
				Plan: validPlan, Rejected: lifecycle.ControlNotFound,
			},
			wantErr: true,
		},
		"unknown rejection": {
			decision: FunctionCatalogDecision{Rejected: 418},
			wantErr:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gotErr := test.decision.validate() != nil
			require.EqualValues(t, test.wantErr, gotErr)
		})
	}
}

func TestFunctionInvocationRefValid(t *testing.T) {
	tests := map[string]struct {
		ref  FunctionInvocationRef
		want bool
	}{
		"valid":           {ref: FunctionInvocationRef{Slot: 1, Generation: 1}, want: true},
		"missing slot":    {ref: FunctionInvocationRef{Generation: 1}},
		"missing version": {ref: FunctionInvocationRef{Slot: 1}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := test.ref.Valid()
			require.EqualValues(t, test.want, got)
		})
	}
}
