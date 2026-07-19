package contract

// HotpathGate binds one published owner to enforceable deterministic production
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
			"TestKernelLoopContinuesPendingTaskStartsAcrossServiceQuanta",
			"TestKernelAsyncEventServiceQuantumIsPhaseBalancedAndBounded",
			"TestKernelShutdownCancelsInitialOperationSweepBeforePendingTaskDispatch",
		},
		Benchmark: "BenchmarkBKernelMixedTurn",
	},
	{
		OwnerID: "B-O-00006",
		Package: "./plugin/agent/jobmgr/lifecycle",
		Tests: []string{
			"TestTaskSupervisorDynamicPopulationAndGenerationCheckedReuse",
			"TestTaskSupervisorDispatchRotatesPendingClasses",
			"TestTaskSupervisorFrameworkControlStartsWithManyActiveGenericTasks",
			"TestTaskSupervisorRetainedTimeoutCountAndSaturationLatch",
			"TestPendingTaskRequestPopulationGrowsBeyondFormerLimit",
			"TestLongLivedJobPermitPopulationGrowsBeyondFormerLimit",
			"TestLongLivedPermitConservesAdmittedBGEFacets",
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
			"TestFunctionCatalogRetainsCleanupExecutionStorageUntilCompletion",
			"TestFunctionCatalogAbortRetainsInitializedCleanupExecutionStorage",
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
			"TestProductionProcessChargesCatalogStorageUntilFinalClose",
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

var bmM003HotpathRows = [...]HotpathGate{
	{
		OwnerID: "B-O-00027",
		Package: "./plugin/agent/secrets/secretstore",
		Tests: []string{
			"TestSecretStoreLeaseRetirementAndDynamicPopulation",
			"TestSecretStoreCloseRejectsRetainedScope",
		},
		Benchmark: "BenchmarkBSecretStoreLease",
	},
	{
		OwnerID: "B-O-00028",
		Package: "./plugin/agent/secrets/secretstore",
		Tests: []string{
			"TestSecretStorePreparationOwnershipRegressions",
		},
		Benchmark: "BenchmarkBSecretMutationControl",
	},
	{
		OwnerID: "B-O-00029",
		Package: "./plugin/agent/jobmgr/secrets",
		Tests: []string{
			"TestSecretDependencyIndexTracksAcknowledgedPostimages",
		},
		Benchmark: "BenchmarkBSecretDependencyLookup",
	},
	{
		OwnerID: "B-O-00030",
		Package: "./plugin/agent/jobmgr/secrets",
		Tests: []string{
			"TestSecretRestartCommandCommitsWithoutDependentsOrCompositeScope",
			"TestSecretRestartCommandReportsFailedPrecommitRestoration",
			"TestSecretRestartCommandRestoresStopAcknowledgedDuringCancellation",
			"TestSecretRestartCommandRedactsAppliedRestartFailure",
		},
		Benchmark: "BenchmarkBSecretRestart",
	},
}

var bmM004HotpathRows = [...]HotpathGate{
	{
		OwnerID: "B-O-00032",
		Package: "./plugin/agent/jobmgr/discovery",
		Tests: []string{
			"TestDecisionIndexAcknowledgesSelectionAndFallback",
			"TestDecisionIndexFailureKeepsLastAcknowledgedSelection",
			"TestDecisionIndexReconcilesOnlyChangedSourceRecords",
			"TestDecisionIndexHasNoFixedPopulationCeiling",
		},
		Benchmark: "BenchmarkBDecisionIndexApply",
	},
	{
		OwnerID: "B-O-00033",
		Package: "./plugin/agent/discovery",
		Tests: []string{
			"TestPipelineGenerationConstruction",
			"TestPipelineGenerationRunsNamedProvidersAndAggregates",
			"TestPipelineGenerationPropagatesApplyFailure",
			"TestPipelineGenerationHasNoFixedProviderPopulationCeiling",
		},
		Benchmark: "BenchmarkBPipelineGenerationRun",
	},
}

func BMM002HotpathGates() []HotpathGate {
	return cloneHotpathGates(bmM002HotpathRows[:])
}

func BMM003HotpathGates() []HotpathGate {
	gates := cloneHotpathGates(bmM002HotpathRows[:])
	for index := range gates {
		switch gates[index].OwnerID {
		case "B-O-00001":
			gates[index].Tests = append(
				gates[index].Tests,
				"TestAdmissionSuspensionPreservesRetainedBytes",
			)
		case "B-O-00003":
			gates[index].Tests = append(
				gates[index].Tests,
				"TestCompositeChildContinuationPrecedesRunnableTargetLaneWork",
				"TestCompositeFenceDefersConflictingAdmissionButNotUnrelatedWork",
			)
		}
	}
	return append(gates, cloneHotpathGates(bmM003HotpathRows[:])...)
}

func BMM002HotpathProofs() []ComponentProof {
	return hotpathProofs(bmM002HotpathRows[:])
}

func BMM003HotpathProofs() []ComponentProof {
	return hotpathProofs(BMM003HotpathGates())
}

func BMM004HotpathGates() []HotpathGate {
	gates := BMM003HotpathGates()
	for index := range gates {
		switch gates[index].OwnerID {
		case "B-O-00006":
			gates[index].Tests = append(
				gates[index].Tests,
				"TestPipelineLongLivedPlanProviderKeys",
				"TestPipelinePermitReleasesDisabledProviderClaim",
			)
		case "B-O-00007":
			gates[index].Tests = append(
				gates[index].Tests,
				"TestInheritedPipelineTasksRequirePermit",
			)
		case "B-O-00036":
			gates[index].Tests = append(
				gates[index].Tests,
				"TestRunGenerationOwnsFrozenDiscoveryChildren",
				"TestProcessCoreRejectsSuccessorAfterDiscoveryProviderMissesJoin",
			)
		}
	}
	return append(gates, cloneHotpathGates(bmM004HotpathRows[:])...)
}

func BMM004HotpathProofs() []ComponentProof {
	return hotpathProofs(BMM004HotpathGates())
}

func cloneHotpathGates(rows []HotpathGate) []HotpathGate {
	gates := make([]HotpathGate, len(rows))
	for index, row := range rows {
		row.Tests = append([]string(nil), row.Tests...)
		gates[index] = row
	}
	return gates
}

func hotpathProofs(rows []HotpathGate) []ComponentProof {
	seen := make(map[ComponentProof]struct{})
	var proofs []ComponentProof
	for _, gate := range rows {
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
