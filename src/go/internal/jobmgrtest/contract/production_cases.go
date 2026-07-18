package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const BMM002RequiredCases = 90

type RuntimeSuite string

const (
	SuiteAgent             RuntimeSuite = "agent"
	SuiteProcess           RuntimeSuite = "process"
	SuiteShippedRoot       RuntimeSuite = "shipped-root"
	SuiteCollectorBoundary RuntimeSuite = "collector-boundary"
	SuiteResolver          RuntimeSuite = "resolver"
)

type ProofKind string

const (
	ProofRuntime   ProofKind = "runtime"
	ProofComponent ProofKind = "component"
	ProofCombined  ProofKind = "runtime+component"
)

type ProductionCase struct {
	ID    string
	Suite RuntimeSuite
	Proof ProofKind
}

var bmM002CaseRows = [...]ProductionCase{
	{ID: "F01.1", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F01.2", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F02.1-v1-synthetic", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F02.1-v2-synthetic", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F02.1-v2-nested-cleanup-boundary", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F02.2", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F02.3", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F03.1", Suite: SuiteAgent, Proof: ProofComponent},
	{ID: "F03.2", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F04.2", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F04.3", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F05.1", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F05.2", Suite: SuiteProcess, Proof: ProofCombined},
	{ID: "F05.3", Suite: SuiteProcess, Proof: ProofRuntime},
	{ID: "F06.1", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F06.2", Suite: SuiteProcess, Proof: ProofCombined},
	{ID: "F07.1", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F07.2", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F08.1", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F08.2", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F08.3", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F08.4", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F08.5", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F08.6", Suite: SuiteAgent, Proof: ProofComponent},
	{ID: "F08.7", Suite: SuiteAgent, Proof: ProofComponent},
	{ID: "F08.8", Suite: SuiteAgent, Proof: ProofComponent},
	{ID: "F10.1", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F10.5", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F10.6", Suite: SuiteResolver, Proof: ProofRuntime},
	{ID: "F10.7", Suite: SuiteCollectorBoundary, Proof: ProofCombined},
	{ID: "F11.1", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F11.2", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F11.3", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F11.4", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F11.5", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F11.6", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F11.7-job-cleanup-capacity", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F12.1", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F12.2", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F12.3", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F12.4", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F14.1", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F14.2", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F14.3", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F14.4", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F14.5", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F14.6", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F14.7", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F14.8", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F14.9", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F14.10", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F14.11", Suite: SuiteAgent, Proof: ProofComponent},
	{ID: "F14.13", Suite: SuiteAgent, Proof: ProofRuntime},
	{ID: "F18.1", Suite: SuiteCollectorBoundary, Proof: ProofCombined},
	{ID: "F18.4", Suite: SuiteCollectorBoundary, Proof: ProofCombined},
	{ID: "F18.5", Suite: SuiteCollectorBoundary, Proof: ProofCombined},
	{ID: "F21.1", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F21.2", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F21.3", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F22.1-agent", Suite: SuiteProcess, Proof: ProofCombined},
	{ID: "F24.1-b-retained-slots-1-3", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.2-b-fourth-saturation", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.3-b-background-saturation", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.4-b-response-disposal", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.5-b-late-result-disposal", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.6-b-lane-release-order", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.7-b-claim-direct-cancel", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.8-b-claim-revalidation-fifo", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.9-b-kernel-priority", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.10-b-kernel-source-rotation", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.11-b-task-source-fairness", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.12-b-catalog-loop-lookup", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.13-b-catalog-atomic-mutation", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.14-b-hotpath-envelope", Suite: SuiteAgent, Proof: ProofComponent},
	{ID: "F24.15-b-cleanup-capacity-execution", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.16-b-shutdown-complexity", Suite: SuiteProcess, Proof: ProofComponent},
	{ID: "F24.18-b-ready-lane-fairness", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.19-b-oldest-fitting-admission", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.20-b-godplugin-terminal", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.21-b-godplugin-nonterminal", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.22-b-godplugin-hup", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.23-b-ibmdplugin-terminal", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.24-b-ibmdplugin-nonterminal", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.25-b-ibmdplugin-hup", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.26-b-scriptsdplugin-terminal", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.27-b-scriptsdplugin-nonterminal", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.28-b-scriptsdplugin-hup", Suite: SuiteShippedRoot, Proof: ProofRuntime},
	{ID: "F24.29-b-task-phase-handshake", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.30-b-task-phase-cancel-shutdown", Suite: SuiteAgent, Proof: ProofCombined},
	{ID: "F24.31-b-timeout-control-frame", Suite: SuiteAgent, Proof: ProofCombined},
}

func BMM002Cases() ([]ProductionCase, error) {
	if len(bmM002CaseRows) != BMM002RequiredCases {
		return nil, errors.New("B-M-002 case contract: row count differs")
	}
	knownSuites := map[RuntimeSuite]struct{}{
		SuiteAgent: {}, SuiteProcess: {}, SuiteShippedRoot: {},
		SuiteCollectorBoundary: {}, SuiteResolver: {},
	}
	knownProofs := map[ProofKind]struct{}{
		ProofRuntime: {}, ProofComponent: {}, ProofCombined: {},
	}
	seen := make(map[string]struct{}, len(bmM002CaseRows))
	cases := make([]ProductionCase, len(bmM002CaseRows))
	for index, row := range bmM002CaseRows {
		if row.ID == "" {
			return nil, errors.New("B-M-002 case contract: empty ID")
		}
		if _, exists := seen[row.ID]; exists {
			return nil, errors.New("B-M-002 case contract: duplicate ID")
		}
		if _, exists := knownSuites[row.Suite]; !exists {
			return nil, errors.New("B-M-002 case contract: unknown suite")
		}
		if _, exists := knownProofs[row.Proof]; !exists {
			return nil, errors.New("B-M-002 case contract: unknown proof")
		}
		seen[row.ID] = struct{}{}
		cases[index] = row
	}
	return cases, nil
}

func BMM002CaseSHA256() (string, error) {
	cases, err := BMM002Cases()
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	for _, productionCase := range cases {
		_, _ = digest.Write([]byte(productionCase.ID))
		_, _ = digest.Write([]byte{'\t'})
		_, _ = digest.Write([]byte(productionCase.Suite))
		_, _ = digest.Write([]byte{'\t'})
		_, _ = digest.Write([]byte(productionCase.Proof))
		_, _ = digest.Write([]byte{'\n'})
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
