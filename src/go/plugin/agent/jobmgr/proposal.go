// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import "errors"

// ProposalRejection marks an external proposal that this run cannot apply.
// It is not a kernel, ownership, cancellation, or infrastructure failure.
type ProposalRejection struct {
	cause error
}

func (pr *ProposalRejection) Error() string {
	if pr == nil || pr.cause == nil {
		return "jobmgr: proposal rejected"
	}
	return "jobmgr: proposal rejected: " + pr.cause.Error()
}

func (pr *ProposalRejection) Unwrap() error {
	if pr == nil {
		return nil
	}
	return pr.cause
}

func RejectProposal(cause error) error {
	if cause == nil {
		cause = errors.New("unspecified proposal rejection")
	}
	if _, ok := errors.AsType[*ProposalRejection](cause); ok {
		return cause
	}
	return &ProposalRejection{cause: cause}
}

func IsProposalRejection(err error) bool {
	_, ok := errors.AsType[*ProposalRejection](err)
	return ok
}
