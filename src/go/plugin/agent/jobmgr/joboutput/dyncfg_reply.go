// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// This file owns every result of the collector-job DynCfg Function.
//
// The Netdata daemon saves add, update, enable, disable and remove only on a
// 2xx reply and replays saved configurations on every plugin start, so for
// those commands the reply code states adoption: 2xx when the job graph holds
// the requested state, non-2xx when nothing changed. Job health travels in
// the CONFIG status frame and the message. Rejections leave state unchanged
// and take their code from the failure class; adopted changes take their code
// from the command and the resulting status; mutation replies are sealed only
// once Apply knows what it committed.

// jobFailureClass says why a collector-job command did not end with a running
// job. It picks the code of a rejection and of an informative reply.
type jobFailureClass uint8

const (
	failureNone           jobFailureClass = iota
	failureBadRequest                     // malformed command or arguments
	failureForbidden                      // command not allowed in the record's state
	failureNotFound                       // unknown module or job
	failureNotAllowed                     // command not supported for this job
	failureNotImplemented                 // unknown command
	failureInternal                       // plugin-side error
	failureInvalid                        // the configuration cannot be applied
	failurePermanent                      // collector failure that retrying cannot fix
	failureUnclassified                   // collector failure without a class
	failureTemporary                      // failure that may clear on its own
	failureUnavailable                    // the job cannot be worked on right now
)

func (c jobFailureClass) code() int {
	switch c {
	case failureBadRequest, failureInvalid:
		return 400
	case failureForbidden:
		return 403
	case failureNotFound:
		return 404
	case failureNotAllowed:
		return 405
	case failurePermanent, failureUnclassified:
		return 422
	case failureNotImplemented:
		return 501
	case failureTemporary, failureUnavailable:
		return 503
	default:
		return 500
	}
}

// jobFailure is a classified failure with the message shown to the user.
type jobFailure struct {
	class   jobFailureClass
	message string
}

// preparationFailure classifies an activation or validation error found
// before the incumbent is touched. It reports false for an operational error,
// which the caller returns so the command fails without a state change.
func preparationFailure(activation activationFailure, busy, deadline, quarantined string) (jobFailure, bool) {
	switch activation.kind {
	case activationFailureBusy, activationFailureStaleStore, activationFailureSuperseded:
		return jobFailure{class: failureUnavailable, message: busy}, true
	case activationFailureDeadline:
		return jobFailure{class: failureUnavailable, message: deadline}, true
	case activationFailureQuarantined:
		return jobFailure{class: failureUnavailable, message: quarantined}, true
	case activationFailureTransient:
		return jobFailure{class: failureTemporary, message: activation.err.Error()}, true
	case activationFailureProposal:
		return jobFailure{class: failureInvalid, message: activation.err.Error()}, true
	default:
		return jobFailure{}, false
	}
}

// collectorFailure classifies a failed candidate probe or runtime startup.
func collectorFailure(failure *autoDetectionFailure, format string) jobFailure {
	class := failureUnclassified
	switch {
	case failure.class == collectorapi.LifecycleErrorPermanent:
		class = failurePermanent
	case failure.class == collectorapi.LifecycleErrorTemporary, failure.runtime:
		class = failureTemporary
	}
	return jobFailure{class: class, message: fmt.Sprintf(format, failure.cause)}
}

// testFailure classifies a failed test: jobmgr could not run it, the
// configuration cannot apply, or the collector failed with its class.
func testFailure(err error) jobFailure {
	failure := jobFailure{message: err.Error()}
	switch classifyActivationError(err).kind {
	case activationFailureBusy,
		activationFailureStaleStore,
		activationFailureSuperseded,
		activationFailureDeadline,
		activationFailureQuarantined:
		failure.class = failureUnavailable
	case activationFailureProposal:
		failure.class = failureInvalid
	case activationFailureTransient:
		failure.class = failureTemporary
	default:
		switch collectorapi.ClassifyLifecycleError(err) {
		case collectorapi.LifecycleErrorPermanent:
			failure.class = failurePermanent
		case collectorapi.LifecycleErrorTemporary:
			failure.class = failureTemporary
		default:
			failure.class = failureUnclassified
		}
	}
	return failure
}

// configurationFailure classifies a failed get.
func configurationFailure(err error) jobFailure {
	switch classifyActivationError(err).kind {
	case activationFailureBusy, activationFailureDeadline, activationFailureQuarantined:
		return jobFailure{class: failureUnavailable, message: err.Error()}
	default:
		return jobFailure{class: failureInternal, message: err.Error()}
	}
}

func notImplementedFailure(command dyncfg.Command) jobFailure {
	return jobFailure{
		class:   failureNotImplemented,
		message: fmt.Sprintf("Function '%s' command '%s' is not implemented.", DynCfgFunctionName, command),
	}
}

// adoptsFailure reports whether a command adopts a candidate that failed: only
// when the plugin will retry it, so the adopted state converges by itself.
// Any other failure is rejected with the job graph and incumbent unchanged.
func adoptsFailure(failure *autoDetectionFailure) bool {
	return failure != nil && failure.retry && failure.retryAfter > 0
}

// jobReply is the reply intent of a transaction that changes the job graph.
// An empty command marks response-free internal work.
type jobReply struct {
	command dyncfg.Command
	status  dyncfg.Status // status of the adopted postimage; empty when the change removes the job
	failure jobFailure    // why an adopted change ended Failed or removed
}

// internalReply is the reply intent of response-free internal work.
func internalReply() jobReply {
	return jobReply{}
}

func adoptedReply(command dyncfg.Command, status dyncfg.Status, failure jobFailure) jobReply {
	return jobReply{command: command, status: status, failure: failure}
}

// transactionOutcome is what Apply did with a prepared graph change.
type transactionOutcome struct {
	fallback   *jobFailure // an install fallback committed a Failed postimage for this reason
	rolledBack bool        // nothing was committed; run retirement rolled the change back
}

// seal builds the reply from what Apply did.
func (r jobReply) seal(outcome transactionOutcome) lifecycle.SealedResult {
	switch {
	case r.command == "":
		return noResponseResult()
	case outcome.rolledBack:
		return rejectedResult(jobFailure{
			class:   failureUnavailable,
			message: "the plugin is restarting; retry the command.",
		})
	case outcome.fallback != nil:
		return adoptedResult(r.command, dyncfg.StatusFailed, *outcome.fallback)
	default:
		return adoptedResult(r.command, r.status, r.failure)
	}
}

// adoptedResult answers a command whose requested state the job graph holds.
// Persisted commands answer 2xx: 200 for running, disabled and removed jobs,
// 202 for accepted and failed ones. restart is not persisted and answers 200
// only when the restarted job runs. Accepted is internal admission state; the
// outer restart handler waits for its exact activation before answering.
func adoptedResult(command dyncfg.Command, status dyncfg.Status, failure jobFailure) lifecycle.SealedResult {
	if command == dyncfg.CommandRestart {
		if status == dyncfg.StatusAccepted {
			return noResponseResult()
		}
		if status == dyncfg.StatusRunning {
			return messageResult(200, "")
		}
		return messageResult(failure.class.code(), failure.message)
	}
	code := 200
	switch {
	case status == dyncfg.StatusAccepted, status == dyncfg.StatusFailed:
		code = 202
	case status == "" && command != dyncfg.CommandRemove:
		// A plain stock job whose detection failed is removed.
		code = 202
	}
	return messageResult(code, failure.message)
}

// satisfiedResult answers a command whose requested state already held.
func satisfiedResult(status dyncfg.Status) lifecycle.SealedResult {
	if status == dyncfg.StatusAccepted {
		return messageResult(202, "")
	}
	return messageResult(200, "")
}

// rejectedResult answers a command that left every state unchanged.
func rejectedResult(failure jobFailure) lifecycle.SealedResult {
	return messageResult(failure.class.code(), failure.message)
}

// passedResult answers a successful test.
func passedResult() lifecycle.SealedResult {
	return messageResult(200, "")
}

// payloadResult answers schema, get and userconfig.
func payloadResult(contentType string, payload []byte) (lifecycle.SealedResult, error) {
	return lifecycle.NewSealedResult(200, contentType, payload)
}

// noResponseResult completes response-free internal work.
func noResponseResult() lifecycle.SealedResult {
	return messageResult(204, "")
}

func messageResult(code int, message string) lifecycle.SealedResult {
	result, err := lifecycle.NewSealedResult(
		code,
		"application/json",
		frameworkfunctions.BuildJSONPayload(code, message),
	)
	if err != nil {
		panic(err)
	}
	return result
}

// reject answers a command without changing the job graph or the incumbent.
// It carries no side effects, so no retry, pending start or activation is
// armed for the rejected request.
func (dcjc *DynCfgJobController) reject(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	failure jobFailure,
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.noop(scope, current, permit, rejectedResult(failure))
}

// rejectKeeping is reject plus a status frame restating the kept status, so a
// UI that already showed a transition converges on the unchanged record.
func (dcjc *DynCfgJobController) rejectKeeping(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	failure jobFailure,
	status dyncfg.Status,
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.noop(
		scope,
		current,
		permit,
		rejectedResult(failure),
		dcjc.configStatusCleanup(scope.ID, status),
	)
}

// satisfy answers a command whose requested state already holds.
func (dcjc *DynCfgJobController) satisfy(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	status dyncfg.Status,
	afterApply func(),
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.noopWithAfterApply(
		scope,
		current,
		permit,
		satisfiedResult(status),
		afterApply,
		dcjc.configStatusCleanup(scope.ID, status),
	)
}
