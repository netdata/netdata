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

type runtimePredicateDeclaration struct {
	Suite     RuntimeSuite
	Predicate string
}

// runtimePredicateByCase is the closed executable runtime contract. Entries
// are explicit so adding or reclassifying a case cannot silently inherit its
// case label as runtime credit.
var runtimePredicateByCase = map[string]runtimePredicateDeclaration{
	"F01.1": {
		Suite: SuiteAgent, Predicate: "F01.1",
	},
	"F01.2": {
		Suite: SuiteAgent, Predicate: "F01.2",
	},
	"F02.1-v1-synthetic": {
		Suite: SuiteAgent, Predicate: "F02.1-v1-synthetic",
	},
	"F02.1-v2-synthetic": {
		Suite: SuiteAgent, Predicate: "F02.1-v2-synthetic",
	},
	"F02.1-v2-nested-cleanup-boundary": {
		Suite:     SuiteAgent,
		Predicate: "F02.1-v2-nested-cleanup-boundary",
	},
	"F02.2": {
		Suite: SuiteAgent, Predicate: "F02.2",
	},
	"F02.3": {
		Suite: SuiteAgent, Predicate: "F02.3",
	},
	"F04.2": {
		Suite: SuiteAgent, Predicate: "F04.2",
	},
	"F04.3": {
		Suite: SuiteAgent, Predicate: "F04.3",
	},
	"F05.1": {
		Suite: SuiteAgent, Predicate: "F05.1",
	},
	"F05.2": {
		Suite: SuiteProcess, Predicate: "F05.2",
	},
	"F05.3": {
		Suite: SuiteProcess, Predicate: "F05.3",
	},
	"F06.1": {
		Suite: SuiteShippedRoot, Predicate: "F06.1",
	},
	"F06.2": {
		Suite: SuiteProcess, Predicate: "F06.2",
	},
	"F07.1": {
		Suite: SuiteAgent, Predicate: "F07.1",
	},
	"F07.2": {
		Suite: SuiteAgent, Predicate: "F07.2",
	},
	"F08.1": {
		Suite: SuiteAgent, Predicate: "F08.1",
	},
	"F08.2": {
		Suite: SuiteAgent, Predicate: "F08.2",
	},
	"F08.3": {
		Suite: SuiteAgent, Predicate: "F08.3",
	},
	"F08.4": {
		Suite: SuiteAgent, Predicate: "F08.4",
	},
	"F08.5": {
		Suite: SuiteAgent, Predicate: "F08.5",
	},
	"F10.6": {
		Suite: SuiteResolver, Predicate: "F10.6",
	},
	"F10.7": {
		Suite: SuiteCollectorBoundary, Predicate: "F10.7",
	},
	"F11.1": {
		Suite: SuiteAgent, Predicate: "F11.1",
	},
	"F11.2": {
		Suite: SuiteAgent, Predicate: "F11.2",
	},
	"F11.3": {
		Suite: SuiteAgent, Predicate: "F11.3",
	},
	"F11.4": {
		Suite: SuiteAgent, Predicate: "F11.4",
	},
	"F11.5": {
		Suite: SuiteAgent, Predicate: "F11.5",
	},
	"F11.6": {
		Suite: SuiteAgent, Predicate: "F11.6",
	},
	"F14.1": {
		Suite: SuiteAgent, Predicate: "F14.1",
	},
	"F14.2": {
		Suite: SuiteAgent, Predicate: "F14.2",
	},
	"F14.3": {
		Suite: SuiteAgent, Predicate: "F14.3",
	},
	"F14.4": {
		Suite: SuiteAgent, Predicate: "F14.4",
	},
	"F14.5": {
		Suite: SuiteAgent, Predicate: "F14.5",
	},
	"F14.13": {
		Suite: SuiteAgent, Predicate: "F14.13",
	},
	"F18.1": {
		Suite: SuiteCollectorBoundary, Predicate: "F18.1",
	},
	"F18.5": {
		Suite: SuiteCollectorBoundary, Predicate: "F18.5",
	},
	"F22.1-agent": {
		Suite: SuiteProcess, Predicate: "F22.1-agent",
	},
	"F24.20-b-godplugin-terminal": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.20-b-godplugin-terminal",
	},
	"F24.21-b-godplugin-nonterminal": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.21-b-godplugin-nonterminal",
	},
	"F24.22-b-godplugin-hup": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.22-b-godplugin-hup",
	},
	"F24.23-b-ibmdplugin-terminal": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.23-b-ibmdplugin-terminal",
	},
	"F24.24-b-ibmdplugin-nonterminal": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.24-b-ibmdplugin-nonterminal",
	},
	"F24.25-b-ibmdplugin-hup": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.25-b-ibmdplugin-hup",
	},
	"F24.26-b-scriptsdplugin-terminal": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.26-b-scriptsdplugin-terminal",
	},
	"F24.27-b-scriptsdplugin-nonterminal": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.27-b-scriptsdplugin-nonterminal",
	},
	"F24.28-b-scriptsdplugin-hup": {
		Suite:     SuiteShippedRoot,
		Predicate: "F24.28-b-scriptsdplugin-hup",
	},
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
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestUIDLedgerGrowsAndCloseWorkRemainsBatched"},
	},
	"F08.2": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelGenericFunctionInvocationsOnSameRouteRunConcurrently"},
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelFunctionResourceLanesGrowAndPreserveCrossLaneProgress"},
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
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestUIDLedgerTombstonePopulationGrowsBeyondFormerBoundary"},
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
		{Package: "./plugin/agent/jobmgr/composition", Test: "TestRunGenerationGrowsBeyondFormerJobLimitWithDiscoveryPipeline"},
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
	"F24.11-b-task-class-fairness": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorDispatchRotatesPendingClasses"},
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorFrameworkControlBypassesPendingGenericAtCapacity"},
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelResourceScopedFunctionHasIndependentTaskSchedulingClass"},
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
		{Package: "./plugin/agent/jobmgr/functions", Test: "TestFunctionCatalogLookupAndHandlerLeaseAllocateNothing"},
	},
	"F24.15-b-cleanup-capacity-execution": {
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelRunsTaskCleanupBeforeSlotRelease"},
	},
	"F24.16-b-shutdown-complexity": {
		{Package: "./plugin/agent/jobmgr/lifecycle", Test: "TestTaskSupervisorSealsAndCancelsEveryInheritedContext"},
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelShutdownCancelsOperationsInBoundedTurns"},
		{Package: "./plugin/agent/jobmgr", Test: "TestKernelShutdownVisitsLiveLanesOnceInBoundedTurns"},
		{Package: "./plugin/agent/jobmgr/composition", Test: "TestCloseProcessUIDsObservesShutdownContextBetweenBatches"},
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
	declaration, exists := runtimePredicateByCase[productionCase.ID]
	if !exists || declaration.Suite != productionCase.Suite {
		return ""
	}
	return declaration.Predicate
}

func ValidateEvidenceContract() error {
	cases, err := BMM002Cases()
	if err != nil {
		return err
	}
	byID := make(map[string]ProductionCase, len(cases))
	runtimeKeys := make(map[string]string)
	for _, productionCase := range cases {
		byID[productionCase.ID] = productionCase
		declaration, hasRuntime := runtimePredicateByCase[productionCase.ID]
		if productionCase.Proof == ProofComponent {
			if hasRuntime {
				return fmt.Errorf(
					"B-M-002 evidence: component case %s declares runtime credit",
					productionCase.ID,
				)
			}
		} else if !hasRuntime ||
			declaration.Suite != productionCase.Suite ||
			declaration.Predicate == "" {
			return fmt.Errorf(
				"B-M-002 evidence: case %s lacks an exact runtime declaration",
				productionCase.ID,
			)
		}
		if hasRuntime {
			key := string(declaration.Suite) + "\x00" +
				declaration.Predicate
			if previous, exists := runtimeKeys[key]; exists {
				return fmt.Errorf(
					"B-M-002 evidence: cases %s and %s alias runtime predicate %s",
					previous,
					productionCase.ID,
					declaration.Predicate,
				)
			}
			runtimeKeys[key] = productionCase.ID
		}
		if _, err := EvidenceFor(productionCase); err != nil {
			return err
		}
	}
	for caseID := range runtimePredicateByCase {
		if _, exists := byID[caseID]; !exists {
			return fmt.Errorf(
				"B-M-002 evidence: undeclared runtime case %s",
				caseID,
			)
		}
	}
	for caseID := range componentProofByCase {
		if _, exists := byID[caseID]; !exists {
			return fmt.Errorf(
				"B-M-002 evidence: undeclared component case %s",
				caseID,
			)
		}
	}
	return nil
}
