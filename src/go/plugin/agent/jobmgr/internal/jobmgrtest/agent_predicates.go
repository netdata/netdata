package jobmgrtest

import "context"

type AgentScenario string

var agentRuntimeScenarios = map[AgentScenario]func(context.Context) error{
	"start acknowledgement":      runAgentStartAcknowledgement,
	"start replacement ordering": runAgentStartReplacementOrdering,
	"collector V1 lifecycle": func(ctx context.Context) error {
		return runAgentCollectorLifecycle(ctx, false, true)
	},
	"collector V2 lifecycle": func(ctx context.Context) error {
		return runAgentCollectorLifecycle(ctx, true, true)
	},
	"collector V2 acquired abort": func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, true, false)
	},
	"Function admission closes before lease drain": runAgentFunctionAdmissionClosesBeforeLeaseDrain,
	"Function replacement ordering":                runAgentFunctionReplacementOrdering,
	"collector V1 published abort": func(ctx context.Context) error {
		return runAgentAcquiredAbort(ctx, false, true)
	},
	"blocking stop": runAgentBlockingStop,
	"held handler shutdown quarantines late frame": runAgentHeldHandlerTerminal,
	"UID growth":                             runAgentUIDGrowthBeyondFormerLimit,
	"concurrent same-route Functions":        runAgentConcurrentSameRouteFunctionPopulation,
	"control with large ordinary population": runAgentControlWithLargeOrdinaryPopulation,
	"Function status variants":               runAgentFunctionStatusVariants,
	"Function output fault": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultFunction)
	},
	"config output fault": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultConfig)
	},
	"collector V1 output fault": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultV1)
	},
	"collector V2 output fault": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultV2)
	},
	"runtime output fault": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultRuntime)
	},
	"cleanup output fault": func(ctx context.Context) error {
		return runAgentOutputFaultCut(ctx, outputFaultCleanup)
	},
	"Function header boundaries":  runAgentFunctionHeaderBoundaries,
	"Function body boundaries":    runAgentFunctionBodyBoundaries,
	"Function raw payload":        runAgentFunctionRawPayload,
	"Function timeout boundaries": runAgentFunctionTimeoutBoundaries,
	"Function invalid JSON":       runAgentFunctionInvalidJSON,
	"Function result boundaries":  runAgentFunctionResultBoundaries,
}

func AgentScenarios() map[AgentScenario]struct{} {
	scenarios := make(map[AgentScenario]struct{}, len(agentRuntimeScenarios))
	for scenario := range agentRuntimeScenarios {
		scenarios[scenario] = struct{}{}
	}
	return scenarios
}
