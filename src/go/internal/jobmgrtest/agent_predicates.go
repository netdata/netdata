package jobmgrtest

import "context"

var agentRuntimePredicates = map[string]func(context.Context) error{
	"F01.1": runAgentStartAcknowledgement,
	"F01.2": runAgentStartReplacementOrdering,
	"F02.1-v1-synthetic": func(ctx context.Context) error {
		return runAgentCollectorLifecycle(ctx, false, true)
	},
	"F02.1-v2-synthetic": func(ctx context.Context) error {
		return runAgentCollectorLifecycle(ctx, true, true)
	},
	"F02.1-v2-nested-cleanup-boundary": func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, true, false)
	},
	"F02.2": runAgentFunctionAdmissionClosesBeforeLeaseDrain,
	"F02.3": runAgentFunctionReplacementOrdering,
	"F03.2": runAgentBoundedShutdown,
	"F04.2": func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, true, false)
	},
	"F04.3": func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, false, true)
	},
	"F05.1": runAgentBlockingStop,
	"F07.1": runAgentHeldHandlerShutdown,
	"F07.2": runAgentLateHandlerQuarantine,
	"F08.1": runAgentUIDCapacity,
	"F08.2": runAgentFunctionLaneCapacity,
	"F08.3": runAgentControlAtOrdinaryCapacity,
	"F08.4": runAgentFunctionTerminalVariants,
	"F08.5": runAgentAwaitingTerminalBound,
	"F10.1": runAgentBoundedShutdown,
	"F10.5": runAgentBlockingStop,
	"F11.1": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultFunction)
	},
	"F11.2": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultConfig)
	},
	"F11.3": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultV1)
	},
	"F11.4": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultV2)
	},
	"F11.5": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultRuntime)
	},
	"F11.6": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultCleanup)
	},
	"F11.7-job-cleanup-capacity":          runAgentCleanupCapacity,
	"F12.1":                               runAgentVnodePrepareVisibility,
	"F12.2":                               runAgentVnodeCommit,
	"F12.3":                               runAgentVnodeAbort,
	"F12.4":                               runAgentVnodeAuthorityTransitions,
	"F14.1":                               runAgentFunctionHeaderBoundaries,
	"F14.2":                               runAgentFunctionBodyBoundaries,
	"F14.3":                               runAgentFunctionRawPayload,
	"F14.4":                               runAgentFunctionTimeoutBoundaries,
	"F14.5":                               runAgentFunctionInvalidJSON,
	"F14.6":                               runAgentDeferredPayloadBoundary,
	"F14.7":                               runAgentFunctionEnvelopeBoundary,
	"F14.8":                               runAgentClosedResultVariants,
	"F14.9":                               runAgentResultCodecGoldens,
	"F14.10":                              runAgentResultGraphValidation,
	"F14.13":                              runAgentFunctionResultBoundaries,
	"F21.1":                               runAgentTransferredPreparationFailure,
	"F21.2":                               runAgentFinalPermitOrdering,
	"F21.3":                               runAgentRetainedFailedCleanup,
	"F24.1-b-retained-slots-1-3":          runAgentRetainedSlotProgress,
	"F24.2-b-fourth-saturation":           runAgentFourthSlotFailStop,
	"F24.3-b-background-saturation":       runAgentBackgroundSaturation,
	"F24.4-b-response-disposal":           runAgentResponseBeforeDisposal,
	"F24.5-b-late-result-disposal":        runAgentLateResultDisposal,
	"F24.6-b-lane-release-order":          runAgentLaneReleaseOrdering,
	"F24.7-b-claim-direct-cancel":         runAgentClaimCancellation,
	"F24.8-b-claim-revalidation-fifo":     runAgentClaimRevalidation,
	"F24.9-b-kernel-priority":             runAgentKernelPriority,
	"F24.10-b-kernel-source-rotation":     runAgentSourceRotation,
	"F24.11-b-task-source-fairness":       runAgentTaskSourceFairness,
	"F24.12-b-catalog-loop-lookup":        runAgentCatalogLookup,
	"F24.13-b-catalog-atomic-mutation":    runAgentCatalogMutation,
	"F24.15-b-cleanup-capacity-execution": runAgentCleanupCapacityExecution,
	"F24.18-b-ready-lane-fairness":        runAgentReadyLaneFairness,
	"F24.19-b-oldest-fitting-admission":   runAgentOldestFittingAdmission,
	"F24.29-b-task-phase-handshake":       runAgentTaskPhaseHandshake,
	"F24.30-b-task-phase-cancel-shutdown": runAgentTaskPhaseCancel,
	"F24.31-b-timeout-control-frame":      runAgentTimeoutControlFrame,
}
