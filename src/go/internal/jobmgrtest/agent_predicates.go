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
	"F04.2": func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, true, false)
	},
	"F04.3": func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, false, true)
	},
	"F05.1": runAgentBlockingStop,
	"F07.1": runAgentHeldHandlerShutdown,
	"F07.2": runAgentLateHandlerQuarantine,
	"F08.1": runAgentUIDGrowthBeyondFormerLimit,
	"F08.2": runAgentConcurrentSameRouteFunctionPopulation,
	"F08.3": runAgentControlWithLargeOrdinaryPopulation,
	"F08.4": runAgentFunctionTerminalVariants,
	"F08.5": runAgentAwaitingTerminalBound,
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
	"F14.1":  runAgentFunctionHeaderBoundaries,
	"F14.2":  runAgentFunctionBodyBoundaries,
	"F14.3":  runAgentFunctionRawPayload,
	"F14.4":  runAgentFunctionTimeoutBoundaries,
	"F14.5":  runAgentFunctionInvalidJSON,
	"F14.13": runAgentFunctionResultBoundaries,
}
