package jobmgrtest

import "context"

type AgentScenario string

const (
	AgentStartAcknowledgement               AgentScenario = "start acknowledgement"
	AgentStartReplacementOrdering           AgentScenario = "start replacement ordering"
	AgentCollectorV1Lifecycle               AgentScenario = "collector V1 lifecycle"
	AgentCollectorV2Lifecycle               AgentScenario = "collector V2 lifecycle"
	AgentCollectorV2AcquiredAbort           AgentScenario = "collector V2 acquired abort"
	AgentFunctionAdmissionClosesBeforeDrain AgentScenario = "Function admission closes before lease drain"
	AgentFunctionReplacementOrdering        AgentScenario = "Function replacement ordering"
	AgentCollectorV1PublishedAbort          AgentScenario = "collector V1 published abort"
	AgentBlockingStop                       AgentScenario = "blocking stop"
	AgentHeldHandlerShutdown                AgentScenario = "held handler shutdown"
	AgentLateHandlerQuarantine              AgentScenario = "late handler quarantine"
	AgentUIDGrowth                          AgentScenario = "UID growth"
	AgentConcurrentSameRouteFunctions       AgentScenario = "concurrent same-route Functions"
	AgentControlWithLargeOrdinaryPopulation AgentScenario = "control with large ordinary population"
	AgentFunctionTerminalVariants           AgentScenario = "Function terminal variants"
	AgentAwaitingTerminalBound              AgentScenario = "awaiting terminal bound"
	AgentFunctionOutputFault                AgentScenario = "Function output fault"
	AgentConfigOutputFault                  AgentScenario = "config output fault"
	AgentCollectorV1OutputFault             AgentScenario = "collector V1 output fault"
	AgentCollectorV2OutputFault             AgentScenario = "collector V2 output fault"
	AgentRuntimeOutputFault                 AgentScenario = "runtime output fault"
	AgentCleanupOutputFault                 AgentScenario = "cleanup output fault"
	AgentFunctionHeaderBoundaries           AgentScenario = "Function header boundaries"
	AgentFunctionBodyBoundaries             AgentScenario = "Function body boundaries"
	AgentFunctionRawPayload                 AgentScenario = "Function raw payload"
	AgentFunctionTimeoutBoundaries          AgentScenario = "Function timeout boundaries"
	AgentFunctionInvalidJSON                AgentScenario = "Function invalid JSON"
	AgentFunctionResultBoundaries           AgentScenario = "Function result boundaries"
)

var agentRuntimeScenarios = map[AgentScenario]func(context.Context) error{
	AgentStartAcknowledgement:     runAgentStartAcknowledgement,
	AgentStartReplacementOrdering: runAgentStartReplacementOrdering,
	AgentCollectorV1Lifecycle: func(ctx context.Context) error {
		return runAgentCollectorLifecycle(ctx, false, true)
	},
	AgentCollectorV2Lifecycle: func(ctx context.Context) error {
		return runAgentCollectorLifecycle(ctx, true, true)
	},
	AgentCollectorV2AcquiredAbort: func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, true, false)
	},
	AgentFunctionAdmissionClosesBeforeDrain: runAgentFunctionAdmissionClosesBeforeLeaseDrain,
	AgentFunctionReplacementOrdering:        runAgentFunctionReplacementOrdering,
	AgentCollectorV1PublishedAbort: func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, false, true)
	},
	AgentBlockingStop:                       runAgentBlockingStop,
	AgentHeldHandlerShutdown:                runAgentHeldHandlerShutdown,
	AgentLateHandlerQuarantine:              runAgentLateHandlerQuarantine,
	AgentUIDGrowth:                          runAgentUIDGrowthBeyondFormerLimit,
	AgentConcurrentSameRouteFunctions:       runAgentConcurrentSameRouteFunctionPopulation,
	AgentControlWithLargeOrdinaryPopulation: runAgentControlWithLargeOrdinaryPopulation,
	AgentFunctionTerminalVariants:           runAgentFunctionTerminalVariants,
	AgentAwaitingTerminalBound:              runAgentAwaitingTerminalBound,
	AgentFunctionOutputFault: func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultFunction)
	},
	AgentConfigOutputFault: func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultConfig)
	},
	AgentCollectorV1OutputFault: func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultV1)
	},
	AgentCollectorV2OutputFault: func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultV2)
	},
	AgentRuntimeOutputFault: func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultRuntime)
	},
	AgentCleanupOutputFault: func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultCleanup)
	},
	AgentFunctionHeaderBoundaries:  runAgentFunctionHeaderBoundaries,
	AgentFunctionBodyBoundaries:    runAgentFunctionBodyBoundaries,
	AgentFunctionRawPayload:        runAgentFunctionRawPayload,
	AgentFunctionTimeoutBoundaries: runAgentFunctionTimeoutBoundaries,
	AgentFunctionInvalidJSON:       runAgentFunctionInvalidJSON,
	AgentFunctionResultBoundaries:  runAgentFunctionResultBoundaries,
}
