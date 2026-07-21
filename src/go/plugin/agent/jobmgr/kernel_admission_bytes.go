// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	// This covers aggregate framework ownership for one ordinary operation.
	operationFrameworkAdmissionBytes = int64(4_608)
)

func operationAdmissionBytes(request Request, plan WorkPlan) (int64, error) {
	bytes := operationFrameworkAdmissionBytes
	if request.PayloadCapacity < 0 || request.PayloadCapacity > lifecycle.MaximumInputBodyBytes || request.PayloadCapacity > lifecycle.OrdinaryBudgetBytes-bytes {
		return 0, errors.New("jobmgr kernel: input body does not self-fit admission")
	}
	bytes += request.PayloadCapacity
	if plan.OwnedBytes > lifecycle.OrdinaryBudgetBytes-bytes {
		return 0, errors.New("jobmgr kernel: plan-owned bytes do not self-fit admission")
	}
	bytes += plan.OwnedBytes
	if plan.Transaction != nil && plan.Transaction.AllocateSuccessor {
		persistent := plan.Transaction.Permit.Bytes()
		if !validPersistentAdmission(
			plan.Transaction.Permit,
			lifecycle.OrdinaryBudgetBytes-bytes,
		) {
			return 0, errors.New("jobmgr kernel: transaction successor does not self-fit admission")
		}
		bytes += persistent
	}
	addField := func(field string) bool {
		if int64(len(field)) > lifecycle.OrdinaryBudgetBytes-bytes {
			return false
		}
		bytes += int64(len(field))
		return true
	}
	if !addField(request.UID) ||
		!addField(request.LaneKey) ||
		!addField(request.Route) ||
		!addField(request.ContentType) ||
		!addField(request.Permissions) ||
		!addField(request.CallerSource) {
		return 0, errors.New("jobmgr kernel: operation does not self-fit admission")
	}
	for _, field := range request.Args {
		if !addField(field) {
			return 0, errors.New("jobmgr kernel: operation does not self-fit admission")
		}
	}
	for _, field := range plan.Claims {
		if !addField(field) {
			return 0, errors.New("jobmgr kernel: operation does not self-fit admission")
		}
	}
	const requestArgumentAdmissionBytes = int64(16)
	arguments := int64(len(request.Args))
	if arguments > (lifecycle.OrdinaryBudgetBytes-bytes)/requestArgumentAdmissionBytes {
		return 0, errors.New("jobmgr kernel: request arguments do not self-fit admission")
	}
	bytes += arguments * requestArgumentAdmissionBytes
	const authorityClaimEdgeAdmissionBytes = int64(96)
	authorityClaimEdges := int64(len(plan.Claims))
	if authorityClaimEdges > (lifecycle.OrdinaryBudgetBytes-bytes)/authorityClaimEdgeAdmissionBytes {
		return 0, errors.New("jobmgr kernel: claim edges do not self-fit admission")
	}
	bytes += authorityClaimEdges * authorityClaimEdgeAdmissionBytes
	return bytes, nil
}

func validPersistentAdmission(
	plan lifecycle.LongLivedPlan,
	available int64,
) bool {
	if available < 0 {
		return false
	}
	switch plan.Class() {
	case lifecycle.LongLivedPipeline:
		return plan.Bytes() == 0
	case lifecycle.LongLivedJob, lifecycle.LongLivedSecretStore:
		return plan.Bytes() > 0 && plan.Bytes() <= available
	default:
		return false
	}
}

func (ck *CommandKernel) abortRequestInputBody(request Request) error {
	if request.InputBodyToken == 0 {
		return nil
	}
	wake, err := ck.admission.AbortInputBody(request.InputBodyToken)
	if wake {
		ck.NotifyControlReady()
	}
	return err
}

func (ck *CommandKernel) abortRequestInputBodyWith(
	request Request,
	primary error,
	cleanup ...error,
) error {
	abortErr := ck.abortRequestInputBody(request)
	hasCleanupError := abortErr != nil
	if !hasCleanupError {
		for _, err := range cleanup {
			if err != nil {
				hasCleanupError = true
				break
			}
		}
	}
	if !hasCleanupError {
		return primary
	}
	errs := make([]error, 0, len(cleanup)+2)
	errs = append(errs, primary)
	errs = append(errs, cleanup...)
	errs = append(errs, abortErr)
	return errors.Join(errs...)
}

func operationResultAdmissionBytes(base int64, result lifecycle.ResultPreflight) (int64, error) {
	if base <= 0 || result.PlanBytes < 0 || result.FrameBytes <= 0 {
		return 0, errors.New("jobmgr kernel: invalid result admission terms")
	}
	total := base
	for _, term := range []int64{result.PlanBytes, result.FrameBytes} {
		if term > lifecycle.OrdinaryBudgetBytes-total {
			return 0, fmt.Errorf("%w: result does not self-fit ordinary budget", lifecycle.ErrFunctionResultTooLarge)
		}
		total += term
	}
	return total, nil
}

func sourceIndex(source lifecycle.Source) int {
	if source == lifecycle.SourceJobManager {
		return 0
	}
	return 1
}

func sourceForIndex(index int) lifecycle.Source {
	if index == 0 {
		return lifecycle.SourceJobManager
	}
	return lifecycle.SourceFunction
}

func otherSource(source lifecycle.Source) lifecycle.Source {
	if source == lifecycle.SourceJobManager {
		return lifecycle.SourceFunction
	}
	return lifecycle.SourceJobManager
}
