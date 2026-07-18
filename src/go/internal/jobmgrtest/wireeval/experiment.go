package wireeval

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/oracle"
)

const maximumPerformanceRunDuration = 5 * time.Minute

type ExperimentSpec struct {
	Baseline          ChildSpec
	Production        ChildSpec
	EvidenceDirectory string
	Progress          func(completed, total int)
}

type Experiment struct {
	EnvironmentSHA256 string
	Summaries         []oracle.RunSummary
	Result            oracle.ExperimentResult
}

type exactRead func([]byte) (int, error)

type experimentDeps struct {
	readFull          exactRead
	deriveEnvironment func(ChildSpec, ChildSpec) (string, error)
	run               func(context.Context, RunSpec) (oracle.RunSummary, error)
}

type pairCoordinate struct {
	workloadID string
	population int
	pair       int
}

func RunExperiment(ctx context.Context, spec ExperimentSpec) (Experiment, error) {
	return runExperiment(ctx, spec, experimentDeps{
		readFull:          cryptoReadFull,
		deriveEnvironment: DeriveEnvironmentSHA256,
		run:               Run,
	})
}

func runExperiment(ctx context.Context, spec ExperimentSpec, deps experimentDeps) (Experiment, error) {
	if spec.Baseline.Executable == "" || spec.Production.Executable == "" {
		return Experiment{}, errors.New("wire evaluator: incomplete experiment specification")
	}
	if deps.readFull == nil || deps.deriveEnvironment == nil || deps.run == nil {
		return Experiment{}, errors.New("wire evaluator: incomplete experiment dependencies")
	}
	environmentSHA256, err := deps.deriveEnvironment(spec.Baseline, spec.Production)
	if err != nil {
		return Experiment{}, err
	}
	var evidence *EvidenceBundle
	if spec.EvidenceDirectory != "" {
		evidence, err = NewEvidenceBundle(spec.EvidenceDirectory)
		if err != nil {
			return Experiment{}, err
		}
	}
	nonces, err := preloadPairNonces(deps.readFull)
	if err != nil {
		return Experiment{}, err
	}
	total := len(contract.PerformanceWorkloads()) * 3 * contract.PerformancePairCount * 2
	summaries := make([]oracle.RunSummary, 0, total)
	for _, workload := range contract.PerformanceWorkloads() {
		for _, population := range []int{1, 32, 256} {
			for pair := range contract.PerformancePairCount {
				baselineFirst, err := contract.BaselineRunsFirst(pair)
				if err != nil {
					return Experiment{}, err
				}
				nonce := nonces[pairCoordinate{workload.ID, population, pair}]
				schedule, err := contract.BuildPerformanceSchedule(workload.ID, nonce)
				if err != nil {
					return Experiment{}, err
				}
				runSide := func(side oracle.Side, child ChildSpec, ranFirst bool) error {
					runCtx, cancel := context.WithTimeout(
						ctx,
						maximumPerformanceRunDuration,
					)
					defer cancel()
					summary, err := deps.run(runCtx, RunSpec{
						Child: child, Side: side, WorkloadID: workload.ID, Population: population,
						PairIndex: pair, RanFirst: ranFirst, PairNonce: nonce,
						EnvironmentSHA256: environmentSHA256,
						Schedule:          schedule,
						ObservationSink: func(observation oracle.RunObservation) error {
							if evidence == nil {
								return nil
							}
							return evidence.Record(observation)
						},
					})
					if err != nil {
						return err
					}
					summaries = append(summaries, summary)
					if spec.Progress != nil {
						spec.Progress(len(summaries), total)
					}
					return nil
				}
				if baselineFirst {
					if err := runSide(oracle.SideBaseline, spec.Baseline, true); err != nil {
						return Experiment{}, err
					}
					if err := runSide(oracle.SideProduction, spec.Production, false); err != nil {
						return Experiment{}, err
					}
				} else {
					if err := runSide(oracle.SideProduction, spec.Production, true); err != nil {
						return Experiment{}, err
					}
					if err := runSide(oracle.SideBaseline, spec.Baseline, false); err != nil {
						return Experiment{}, err
					}
				}
			}
		}
	}
	result, err := oracle.EvaluateExperiment(summaries)
	if err != nil {
		return Experiment{}, err
	}
	if evidence != nil {
		if err := evidence.Finalize(environmentSHA256, summaries, result); err != nil {
			return Experiment{}, err
		}
	}
	return Experiment{
		EnvironmentSHA256: environmentSHA256,
		Summaries:         summaries,
		Result:            result,
	}, nil
}

func cryptoReadFull(destination []byte) (int, error) {
	return io.ReadFull(rand.Reader, destination)
}

func preloadPairNonces(readFull exactRead) (map[pairCoordinate][16]byte, error) {
	if readFull == nil {
		return nil, errors.New("wire evaluator: nil pair entropy source")
	}
	count := len(contract.PerformanceWorkloads()) * 3 * contract.PerformancePairCount
	nonces := make(map[pairCoordinate][16]byte, count)
	seen := make(map[[16]byte]pairCoordinate, count)
	for _, workload := range contract.PerformanceWorkloads() {
		for _, population := range []int{1, 32, 256} {
			for pair := range contract.PerformancePairCount {
				coordinate := pairCoordinate{workload.ID, population, pair}
				var nonce [16]byte
				count, err := readFull(nonce[:])
				if err != nil || count != len(nonce) {
					if err == nil {
						err = io.ErrUnexpectedEOF
					}
					return nil, fmt.Errorf("wire evaluator: pair entropy %s/%d/%d: read %d of %d: %w", workload.ID, population, pair, count, len(nonce), err)
				}
				if previous, duplicate := seen[nonce]; duplicate {
					return nil, fmt.Errorf("wire evaluator: pair entropy reused by %s/%d/%d and %s/%d/%d", previous.workloadID, previous.population, previous.pair, workload.ID, population, pair)
				}
				seen[nonce] = coordinate
				nonces[coordinate] = nonce
			}
		}
	}
	return nonces, nil
}
