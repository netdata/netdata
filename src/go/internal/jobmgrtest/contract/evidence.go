package contract

import (
	"errors"
	"fmt"
	"strings"
)

// ComponentProof names one exact package test that supplies facts which are
// intentionally unavailable through the public runtime boundary.
type ComponentProof struct {
	Package string
	Test    string
}

// CaseEvidence binds one required case to its exact black-box predicate and any
// component-level proof needed to close the case honestly.
type CaseEvidence struct {
	RuntimePredicate string
	Components       []ComponentProof
}

var componentProofByCase = map[string][]ComponentProof{
	"F02.1-v1-synthetic": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestManagedJobV1V2JoinBeforeCleanup"},
	},
	"F02.1-v2-synthetic": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestManagedJobV1V2JoinBeforeCleanup"},
	},
	"F02.1-v2-nested-cleanup-boundary": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestFactoryV2RejectsWithExactlyOneCollectorCleanup"},
	},
	"F02.2": {
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestHandlerCleanupOnce"},
	},
	"F02.3": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestManagedJobV1V2JoinBeforeCleanup"},
	},
	"F03.1": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestOperationResponseCommitPrecedesDisposalAcknowledgement"},
	},
	"F03.2": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelFourthBackgroundTimeoutDirtiesWithoutResponseCommit"},
	},
	"F05.2": {
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestProcessIngressKeepsOneReaderAndLinearizesPauseAdoptFence"},
		{Package: "./plugin/agent/jobmgr/composition", Test: "TestProcessCoreRestartsOneInputAndMovesFrameAuthority"},
	},
	"F06.1": {
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestProcessIngressKeepsOneReaderAndLinearizesPauseAdoptFence"},
		{Package: "./plugin/agent/jobmgr/composition", Test: "TestProcessCoreRestartsOneInputAndMovesFrameAuthority"},
	},
	"F06.2": {
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestProcessIngressKeepsOneReaderAndLinearizesPauseAdoptFence"},
	},
	"F07.1": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelStopDrainsCooperativeTask"},
	},
	"F07.2": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelDisposesQueuedNoResponseCapabilityAfterItsDeadline"},
	},
	"F08.1": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestUIDLedgerHasExactFixedCapacity"},
	},
	"F08.2": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelFunctionLaneDepthPreservesCrossLaneProgress"},
	},
	"F08.3": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelSubmissionBacklogCannotStarveStop"},
	},
	"F08.4": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestAdmissionAndUIDExactRelease"},
	},
	"F08.5": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelStartsQueuedCooperativeFunctionAfterItsDeadline"},
	},
	"F08.6": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestUIDLedgerTombstoneCardinalityBoundaries"},
	},
	"F08.7": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestUIDLedgerAdmissionReapsOneBoundedExpiredPrefix"},
	},
	"F08.8": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestUIDLedgerCloseUsesBoundedAcknowledgedBatches"},
	},
	"F10.1": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorRetainedTimeoutCountAndSaturationLatch"},
	},
	"F10.5": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorRetainedTimeoutCountAndSaturationLatch"},
	},
	"F10.7": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestManagedJobV1V2JoinBeforeCleanup"},
	},
	"F11.1": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestFrameOwnerShortWritePoisonsAndRetains"},
	},
	"F11.2": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestFrameOwnerShortWritePoisonsAndRetains"},
	},
	"F11.3": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestFrameOwnerShortWritePoisonsAndRetains"},
	},
	"F11.4": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestFrameOwnerShortWritePoisonsAndRetains"},
	},
	"F11.5": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestFrameOwnerShortWritePoisonsAndRetains"},
	},
	"F11.6": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestFrameOwnerShortWritePoisonsAndRetains"},
	},
	"F11.7-job-cleanup-capacity": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestFrameOwnerControlReservationPrecedesLaterOrdinaryFrame"},
	},
	"F12.1": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestPreparedVNodeFrameHoldsFrameOwnershipThroughMetadataCommit"},
	},
	"F12.2": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestPreparedVNodeFrameHoldsFrameOwnershipThroughMetadataCommit"},
	},
	"F12.3": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestPreparedVNodeFrameCommitFailurePoisonsOwner"},
	},
	"F12.4": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestPreparedVNodeFrameHoldsFrameOwnershipThroughMetadataCommit"},
	},
	"F14.6": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestClosedFunctionResultVariantsHaveExactSizeAppend"},
	},
	"F14.7": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestClosedFunctionResultVariantsHaveExactSizeAppend"},
	},
	"F14.8": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestClosedFunctionValuesRejectInvalidGraphs"},
	},
	"F14.9": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestClosedFunctionResultVariantsHaveExactSizeAppend"},
	},
	"F14.10": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestClosedFunctionValuesRejectInvalidGraphs"},
	},
	"F14.11": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestAdmissionResourceAlgebraBoundaries"},
	},
	"F18.1": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestManagedJobV1V2JoinBeforeCleanup"},
	},
	"F18.4": {
		{Package: "./plugin/agent/jobmgr/composition", Test: "TestRunGenerationPreservesFullJobCapacityWithDiscoveryPipeline"},
	},
	"F18.5": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestJobGenerationRetainsAfterIrrecoverableFailure"},
	},
	"F21.1": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorPreparationPanicReturnsTransferredOwnership"},
	},
	"F21.2": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestJobGenerationPermitReturnLast"},
	},
	"F21.3": {
		{Package: "./plugin/agent/jobmgr/joboutput", Test: "TestJobGenerationRetainsAfterIrrecoverableFailure"},
	},
	"F22.1-agent": {
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestProcessIngressKeepsOneReaderAndLinearizesPauseAdoptFence"},
	},
	"F24.1-b-retained-slots-1-3": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelOneRetainedTimeoutPlusThreeActiveTasksDoesNotDirty"},
	},
	"F24.2-b-fourth-saturation": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelFourthBackgroundTimeoutDirtiesWithoutResponseCommit"},
	},
	"F24.3-b-background-saturation": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorRetainedTimeoutCountAndSaturationLatch"},
	},
	"F24.4-b-response-disposal": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestOperationResponseCommitPrecedesDisposalAcknowledgement"},
	},
	"F24.5-b-late-result-disposal": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelDisposesQueuedNoResponseCapabilityAfterItsDeadline"},
	},
	"F24.6-b-lane-release-order": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelShutdownStopsResourceAfterActiveUserDrains"},
	},
	"F24.7-b-claim-direct-cancel": {
		{Package: "./plugin/agent/jobmgr", Test: "TestClaimAuthorityFIFOAndDirectCancellation"},
	},
	"F24.8-b-claim-revalidation-fifo": {
		{Package: "./plugin/agent/jobmgr", Test: "TestClaimAuthorityFIFOReadersWriters"},
	},
	"F24.9-b-kernel-priority": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelSubmissionBacklogCannotStarveDueDeadline"},
	},
	"F24.10-b-kernel-source-rotation": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelExternalSubmissionServiceRotatesSources"},
	},
	"F24.11-b-task-source-fairness": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorDispatchRotatesPendingSources"},
	},
	"F24.12-b-catalog-loop-lookup": {
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestFunctionCatalogLookupLeaseSameTurn"},
		{Package: "./plugin/agent/jobmgr", Test: "TestFunctionCatalogKernelIntegration"},
	},
	"F24.13-b-catalog-atomic-mutation": {
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestFunctionCatalogAtomicMutation"},
		{Package: "./plugin/agent/jobmgr", Test: "TestFunctionCatalogMutationUsesKernelLoop"},
	},
	"F24.14-b-hotpath-envelope": {
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestFunctionCatalogLookupAllocations"},
	},
	"F24.15-b-cleanup-capacity-execution": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelRunsTaskCleanupBeforeSlotRelease"},
	},
	"F24.16-b-shutdown-complexity": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorSealsAndCancelsEveryInheritedContext"},
	},
	"F24.18-b-ready-lane-fairness": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelReadyLaneFairnessAtBoundaries"},
	},
	"F24.19-b-oldest-fitting-admission": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestAdmissionRadixDomainAndOldestFitting"},
	},
	"F24.29-b-task-phase-handshake": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorChecksPhaseSequenceAndPublishesOwnedResult"},
	},
	"F24.30-b-task-phase-cancel-shutdown": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelStopDrainsCooperativeTask"},
	},
	"F24.31-b-timeout-control-frame": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestFrameOwnerControlReservationPrecedesLaterOrdinaryFrame"},
	},
}

func EvidenceFor(productionCase ProductionCase) (CaseEvidence, error) {
	runtimePredicate := ""
	if productionCase.Proof != ProofComponent {
		runtimePredicate = runtimePredicateFor(productionCase)
		if runtimePredicate == "" {
			return CaseEvidence{}, fmt.Errorf(
				"B-M-002 evidence: case %s has no runtime predicate",
				productionCase.ID,
			)
		}
	}
	components := append(
		[]ComponentProof(nil),
		componentProofByCase[productionCase.ID]...,
	)
	if productionCase.ID == "F24.14-b-hotpath-envelope" {
		components = append(components, BMM002HotpathProofs()...)
	}
	if productionCase.Proof != ProofRuntime && len(components) == 0 {
		return CaseEvidence{}, fmt.Errorf(
			"B-M-002 evidence: case %s has no component proof",
			productionCase.ID,
		)
	}
	for _, proof := range components {
		if !strings.HasPrefix(proof.Package, "./plugin/") ||
			proof.Test == "" ||
			!strings.HasPrefix(proof.Test, "Test") {
			return CaseEvidence{}, errors.New(
				"B-M-002 evidence: invalid component proof",
			)
		}
	}
	return CaseEvidence{
		RuntimePredicate: runtimePredicate,
		Components:       components,
	}, nil
}

func runtimePredicateFor(productionCase ProductionCase) string {
	switch productionCase.Suite {
	case SuiteAgent, SuiteProcess, SuiteShippedRoot,
		SuiteCollectorBoundary, SuiteResolver:
		return productionCase.ID
	}
	return ""
}
