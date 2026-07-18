package contract

// HotpathGate binds one B-M-002 owner to enforceable deterministic production
// gates and one supplementary diagnostic benchmark. Only Tests close the
// owner-specific operation, allocation, fairness, and resource envelope.
type HotpathGate struct {
	OwnerID   string
	Package   string
	Tests     []string
	Benchmark string
}

var bmM002HotpathRows = [...]HotpathGate{
	{
		OwnerID: "B-O-00001",
		Package: "./plugin/agent/jobmgr/lifecycle",
		Tests: []string{
			"TestAdmissionRadixDomainAndOldestFitting",
			"TestAdmissionCleanupFIFOAndDirectCancellation",
			"TestAdmissionResourceAlgebraBoundaries",
			"TestAdmissionPopulationGrowsBeyondFormerLimits",
		},
		Benchmark: "BenchmarkBAdmission",
	},
	{
		OwnerID: "B-O-00002",
		Package: "./plugin/agent/jobmgr/lifecycle",
		Tests: []string{
			"TestOperationResponseCommitPrecedesDisposalAcknowledgement",
		},
		Benchmark: "BenchmarkBOperationTransition",
	},
	{
		OwnerID: "B-O-00003",
		Package: "./plugin/agent/jobmgr",
		Tests: []string{
			"TestKernelGenericFunctionInvocationsOnSameRouteRunConcurrently",
			"TestKernelSameRouteFunctionCancellationIsInvocationLocal",
			"TestKernelFunctionResourceLanesGrowAndPreserveCrossLaneProgress",
			"TestKernelReadyLaneFairnessAtBoundaries",
		},
		Benchmark: "BenchmarkBCommandKernelLaneOps",
	},
	{
		OwnerID: "B-O-00004",
		Package: "./plugin/agent/jobmgr",
		Tests: []string{
			"TestClaimAuthorityLexicographicOrderRetainsAndReleasesPrefix",
			"TestClaimAuthorityFIFOReadersWriters",
			"TestClaimAuthorityCancelAndReleaseAllocateNothing",
			"TestClaimAuthoritySettlementUsesBoundedTurns",
		},
		Benchmark: "BenchmarkClaimAuthorityAcquireCancel",
	},
	{
		OwnerID: "B-O-00005",
		Package: "./plugin/agent/jobmgr",
		Tests: []string{
			"TestKernelSubmissionBacklogCannotStarveDueDeadline",
			"TestKernelExternalSubmissionServiceRotatesSources",
			"TestKernelStartsDueCooperativeRunner",
		},
		Benchmark: "BenchmarkBKernelMixedTurn",
	},
	{
		OwnerID: "B-O-00006",
		Package: "./plugin/agent/jobmgr/lifecycle",
		Tests: []string{
			"TestTaskSupervisorFourSlotsAndGenerationCheckedReuse",
			"TestTaskSupervisorDispatchRotatesPendingSources",
			"TestTaskSupervisorRetainedTimeoutCountAndSaturationLatch",
			"TestPendingTaskRequestPopulationGrowsBeyondFormerLimit",
			"TestLongLivedJobPermitPopulationGrowsBeyondFormerLimit",
		},
		Benchmark: "BenchmarkBTaskSupervisorDispatch",
	},
	{
		OwnerID: "B-O-00007",
		Package: "./plugin/agent/jobmgr/lifecycle",
		Tests: []string{
			"TestTaskSupervisorChecksPhaseSequenceAndPublishesOwnedResult",
			"TestTaskSupervisorPreparationPanicReturnsTransferredOwnership",
			"TestInheritedTasksGrowBeyondFormerDerivedLimit",
		},
		Benchmark: "BenchmarkBTaskChildLaunchCompletion",
	},
	{
		OwnerID: "B-O-00008",
		Package: "./plugin/agent/jobmgr/lifecycle",
		Tests: []string{
			"TestFrameOwnerControlReservationPrecedesLaterOrdinaryFrame",
			"TestFrameOwnerShortWritePoisonsAndRetains",
		},
		Benchmark: "BenchmarkBFrameCommit",
	},
	{
		OwnerID: "B-O-00009",
		Package: "./plugin/agent/jobmgr/lifecycle",
		Tests: []string{
			"TestRunSupervisorOwnsOneBroadcastShutdownBudget",
			"TestRunSupervisorTerminalTruthIsImmutable",
			"TestTaskSupervisorSealsAndCancelsEveryInheritedContext",
		},
		Benchmark: "BenchmarkBRunAck",
	},
	{
		OwnerID: "B-O-00010",
		Package: "./plugin/agent/jobmgr/lifecycle",
		Tests: []string{
			"TestUIDLedgerTombstonePopulationGrowsBeyondFormerBoundary",
			"TestUIDLedgerAdmissionReapsOneBoundedExpiredPrefix",
			"TestUIDLedgerCloseUsesBoundedAcknowledgedBatches",
			"TestActiveFunctionUIDPopulationGrowsBeyondFormerLimit",
		},
		Benchmark: "BenchmarkBUIDAdmission",
	},
	{
		OwnerID: "B-O-00011",
		Package: "./plugin/framework/functions",
		Tests: []string{
			"TestInputCapsuleCommandLineAndTimeoutBoundaries",
			"TestInputCapsulePayloadBoundariesAndResynchronization",
		},
		Benchmark: "BenchmarkBFunctionIngress",
	},
	{
		OwnerID: "B-O-00012",
		Package: "./plugin/agent/jobmgr/functions",
		Tests: []string{
			"TestFunctionCatalogLookupLeaseSameTurn",
			"TestFunctionCatalogAtomicMutation",
			"TestFunctionCatalogBoundedMutationTurns",
			"TestFunctionCatalogLookupAndHandlerLeaseAllocateNothing",
			"TestFunctionCatalogInvocationPopulationGrowsBeyondFormerLimit",
		},
		Benchmark: "BenchmarkBFunctionCatalogLookup",
	},
	{
		OwnerID: "B-O-00013",
		Package: "./plugin/agent/jobmgr/functions",
		Tests: []string{
			"TestHandlerLeaseLifecycle",
			"TestHandlerCleanupOnce",
			"TestFunctionCatalogSharedGenerationUsesRouteLocalLeaseDrain",
			"TestFunctionCatalogReaddsRouteBeforeRetiredLeaseDrains",
			"TestFunctionCatalogLookupAndHandlerLeaseAllocateNothing",
		},
		Benchmark: "BenchmarkBHandlerLease",
	},
	{
		OwnerID: "B-O-00014",
		Package: "./plugin/agent/jobmgr/functions",
		Tests: []string{
			"TestFunctionPublicationSteadyPollAllocatesNothing",
			"TestFunctionPublicationMismatchedAcknowledgementPoisonsAndRetainsHandle",
		},
		Benchmark: "BenchmarkBFunctionPublication",
	},
	{
		OwnerID: "B-O-00015",
		Package: "./plugin/framework/dyncfg",
		Tests: []string{
			"TestDynCfgAtomicIndexes",
			"TestDynCfgGraphOwnsPayloadAndLookupAllocatesNothing",
		},
		Benchmark: "BenchmarkBDynCfgGraph",
	},
	{
		OwnerID: "B-O-00016",
		Package: "./plugin/agent/jobmgr/joboutput",
		Tests: []string{
			"TestFactoryRejectsWithExactlyOneCollectorCleanup",
			"TestFactoryV2RejectsWithExactlyOneCollectorCleanup",
		},
		Benchmark: "BenchmarkBJobFactoryCold",
	},
	{
		OwnerID: "B-O-00017",
		Package: "./plugin/agent/jobmgr/joboutput",
		Tests: []string{
			"TestConfigModuleFactoryCleansEveryAttemptAndPrefersV2",
		},
		Benchmark: "BenchmarkBConfigFactoryCold",
	},
	{
		OwnerID: "B-O-00018",
		Package: "./plugin/agent/jobmgr/joboutput",
		Tests: []string{
			"TestJobGenerationV1V2",
			"TestJobGenerationPermitReturnLast",
			"TestSchedulerWarmTickDoesNotAllocate",
			"TestSchedulerGrowsBeyondFormerActiveJobLimit",
		},
		Benchmark: "BenchmarkBJobGenerationLookup",
	},
	{
		OwnerID: "B-O-00019",
		Package: "./plugin/framework/jobruntime",
		Tests: []string{
			"TestV1RuntimeCycleAndStop",
		},
		Benchmark: "BenchmarkBV1Cycle",
	},
	{
		OwnerID: "B-O-00020",
		Package: "./plugin/framework/jobruntime",
		Tests: []string{
			"TestV2RuntimeCycleAndRunnerStop",
			"TestJobV2RunnerStopWaitsBeforeCleanup",
		},
		Benchmark: "BenchmarkBV2Cycle",
	},
	{
		OwnerID: "B-O-00021",
		Package: "./plugin/framework/jobruntime",
		Tests: []string{
			"TestJobV2HostScopeScenarios",
		},
		Benchmark: "BenchmarkBV2ScopeRegistry",
	},
	{
		OwnerID: "B-O-00022",
		Package: "./plugin/framework/jobruntime",
		Tests: []string{
			"TestJobV2HostScopeScenarios",
			"TestJobV2CleanupUsesPreModuleCleanupSnapshotForStaleSuppression",
		},
		Benchmark: "BenchmarkBV2HostState",
	},
	{
		OwnerID: "B-O-00023",
		Package: "./plugin/agent/jobmgr/joboutput",
		Tests: []string{
			"TestPreparedVNodeFrameHoldsFrameOwnershipThroughMetadataCommit",
			"TestPreparedVNodeFrameCommitFailurePoisonsOwner",
		},
		Benchmark: "BenchmarkBVnodeFrame",
	},
	{
		OwnerID: "B-O-00024",
		Package: "./plugin/framework/jobruntime",
		Tests: []string{
			"TestUnchangedJobVnodeRevisionDoesNotAllocate",
			"TestJobV2_PullVnodeUpdateDuringCollectAppliesOnNextCycle",
		},
		Benchmark: "BenchmarkBJobVNodeSnapshot",
	},
	{
		OwnerID: "B-O-00025",
		Package: "./plugin/agent/secrets/resolver",
		Tests: []string{
			"TestResolverAtomicRollback",
			"TestResolverBounds",
		},
		Benchmark: "BenchmarkBResolverTraversal",
	},
	{
		OwnerID: "B-O-00026",
		Package: "./plugin/agent/secrets/secretstore",
		Tests: []string{
			"TestSecretCreatorCatalogLookupAllocatesNothing",
		},
		Benchmark: "BenchmarkBSecretCreatorLookup",
	},
	{
		OwnerID: "B-O-00031",
		Package: "./plugin/agent/discovery",
		Tests: []string{
			"TestDiscoveryProviderCatalogLookupAllocatesNothing",
		},
		Benchmark: "BenchmarkBDiscoveryFactoryLookup",
	},
	{
		OwnerID: "B-O-00034",
		Package: "./plugin/agent/jobmgr/discovery",
		Tests: []string{
			"TestVNodeConfigAtomicRevisions",
			"TestVNodeConfigBounds",
		},
		Benchmark: "BenchmarkBVNodeConfigLookup",
	},
	{
		OwnerID: "B-O-00035",
		Package: "./plugin/framework/vnoderegistry",
		Tests: []string{
			"TestMetadataLeaseCommitAbort",
			"TestMetadataHistoryIsBounded",
		},
		Benchmark: "BenchmarkBMetadataLease",
	},
	{
		OwnerID: "B-O-00036",
		Package: "./plugin/agent/jobmgr/composition",
		Tests: []string{
			"TestProcessCoreRestartsOneInputAndMovesFrameAuthority",
			"TestRunGenerationGrowsBeyondFormerJobLimitWithDiscoveryPipeline",
		},
		Benchmark: "BenchmarkBProcessRestart",
	},
	{
		OwnerID: "B-O-00037",
		Package: "./plugin/agent/jobmgr/composition",
		Tests: []string{
			"TestModuleCatalogLookupAllocatesNothing",
		},
		Benchmark: "BenchmarkBModuleLookup",
	},
}

func BMM002HotpathGates() []HotpathGate {
	gates := make([]HotpathGate, len(bmM002HotpathRows))
	for index, row := range bmM002HotpathRows {
		row.Tests = append([]string(nil), row.Tests...)
		gates[index] = row
	}
	return gates
}

func BMM002HotpathProofs() []ComponentProof {
	seen := make(map[ComponentProof]struct{})
	var proofs []ComponentProof
	for _, gate := range bmM002HotpathRows {
		for _, test := range gate.Tests {
			proof := ComponentProof{
				Package: gate.Package,
				Test:    test,
			}
			if _, exists := seen[proof]; exists {
				continue
			}
			seen[proof] = struct{}{}
			proofs = append(proofs, proof)
		}
	}
	return proofs
}

func BMM001StructuralProofs() []ComponentProof {
	return []ComponentProof{
		{
			Package: "./plugin/agent/jobmgr",
			Test:    "TestActiveArchitecturePackages",
		},
		{
			Package: "./plugin/agent/jobmgr",
			Test:    "TestProductionConstructionChain",
		},
	}
}
