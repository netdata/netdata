package oracle

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
)

const (
	studentT95 = 1.7613101357748564
	UCBLimit   = 1.10
)

type Gate struct {
	WorkloadID string
	Population int
	Metric     Metric
	UCB        float64
	Pass       bool
}

type ExperimentResult struct {
	Gates []Gate
	Pass  bool
}

func NearestRankP99(samples []int64) (int64, error) {
	values, err := nearestRanks(samples, 99)
	if err != nil {
		return 0, err
	}
	return values[0], nil
}

func nearestRanks(samples []int64, percentiles ...int) ([]int64, error) {
	if len(samples) == 0 {
		return nil, errors.New("empty percentile population")
	}
	ordered := append([]int64(nil), samples...)
	for _, sample := range ordered {
		if sample < 0 {
			return nil, errors.New("negative latency sample")
		}
	}
	for _, percentile := range percentiles {
		if percentile < 1 || percentile > 100 {
			return nil, fmt.Errorf("percentile %d outside 1..100", percentile)
		}
	}
	slices.Sort(ordered)
	values := make([]int64, len(percentiles))
	for index, percentile := range percentiles {
		rank := (percentile*len(ordered) + 99) / 100
		values[index] = ordered[rank-1]
	}
	return values, nil
}

func PairedUpperBound(baseline, production []float64) (float64, error) {
	if len(baseline) != contract.PerformancePairCount || len(production) != contract.PerformancePairCount {
		return 0, fmt.Errorf(
			"got %d baseline and %d production samples, want %d each",
			len(baseline),
			len(production),
			contract.PerformancePairCount,
		)
	}
	ratios := make([]float64, contract.PerformancePairCount)
	var sum float64
	for index := range baseline {
		if !finitePositive(baseline[index]) || !finitePositive(production[index]) {
			return 0, fmt.Errorf("invalid pair %d sample", index)
		}
		ratios[index] = math.Log(production[index] / baseline[index])
		if math.IsInf(ratios[index], 0) || math.IsNaN(ratios[index]) {
			return 0, fmt.Errorf("invalid pair %d log ratio", index)
		}
		sum += ratios[index]
	}
	mean := sum / contract.PerformancePairCount
	var squared float64
	for _, ratio := range ratios {
		delta := ratio - mean
		squared += delta * delta
	}
	standardDeviation := math.Sqrt(squared / (contract.PerformancePairCount - 1))
	upper := math.Exp(mean + studentT95*standardDeviation/math.Sqrt(contract.PerformancePairCount))
	if !finitePositive(upper) {
		return 0, errors.New("non-finite paired upper bound")
	}
	return upper, nil
}

func EvaluateExperiment(summaries []RunSummary) (ExperimentResult, error) {
	wantCount := len(contract.PerformanceWorkloads()) * 3 * contract.PerformancePairCount * 2
	if len(summaries) != wantCount {
		return ExperimentResult{}, fmt.Errorf("evaluator oracle: got %d run summaries, want %d", len(summaries), wantCount)
	}
	type runKey struct {
		workload   string
		population int
		pair       int
		side       Side
	}
	indexed := make(map[runKey]RunSummary, wantCount)
	for _, summary := range summaries {
		if _, err := findWorkload(summary.WorkloadID); err != nil {
			return ExperimentResult{}, err
		}
		if !validPopulation(summary.Population) {
			return ExperimentResult{}, fmt.Errorf("evaluator oracle: invalid population %d", summary.Population)
		}
		if err := validateRunOrder(summary.Side, summary.PairIndex, summary.RanFirst); err != nil {
			return ExperimentResult{}, err
		}
		if !validDigest(summary.PairNonceSHA256) || !validDigest(summary.EnvironmentSHA256) || !validDigest(summary.TranscriptSHA256) || !validDigest(summary.EqualitySHA256) {
			return ExperimentResult{}, errors.New("evaluator oracle: invalid summary digest")
		}
		if err := validateMetrics(summary.Metrics); err != nil {
			return ExperimentResult{}, fmt.Errorf("evaluator oracle: %s/%d/pair-%d/%s: %w", summary.WorkloadID, summary.Population, summary.PairIndex, summary.Side, err)
		}
		key := runKey{summary.WorkloadID, summary.Population, summary.PairIndex, summary.Side}
		if _, exists := indexed[key]; exists {
			return ExperimentResult{}, fmt.Errorf("evaluator oracle: duplicate run summary %#v", key)
		}
		indexed[key] = summary
	}

	result := ExperimentResult{Gates: make([]Gate, 0, len(contract.PerformanceWorkloads())*3*len(metrics)), Pass: true}
	type pairKey struct {
		workload   string
		population int
		pair       int
	}
	nonceOwners := make(map[string]pairKey, len(contract.PerformanceWorkloads())*3*contract.PerformancePairCount)
	for _, workload := range contract.PerformanceWorkloads() {
		for _, population := range []int{1, 32, 256} {
			baselineSamples := make(map[Metric][]float64, len(metrics))
			productionSamples := make(map[Metric][]float64, len(metrics))
			for pair := range contract.PerformancePairCount {
				baselineKey := runKey{workload.ID, population, pair, SideBaseline}
				productionKey := runKey{workload.ID, population, pair, SideProduction}
				baseline, baselineExists := indexed[baselineKey]
				production, productionExists := indexed[productionKey]
				if !baselineExists || !productionExists {
					return ExperimentResult{}, fmt.Errorf("evaluator oracle: missing pair %s/%d/%d", workload.ID, population, pair)
				}
				if baseline.PairNonceSHA256 != production.PairNonceSHA256 ||
					baseline.EnvironmentSHA256 != production.EnvironmentSHA256 ||
					baseline.EqualitySHA256 != production.EqualitySHA256 {
					return ExperimentResult{}, fmt.Errorf("evaluator oracle: pair evidence mismatch %s/%d/%d", workload.ID, population, pair)
				}
				owner := pairKey{workload.ID, population, pair}
				if previous, exists := nonceOwners[baseline.PairNonceSHA256]; exists && previous != owner {
					return ExperimentResult{}, fmt.Errorf("evaluator oracle: pair nonce reused by %#v and %#v", previous, owner)
				}
				nonceOwners[baseline.PairNonceSHA256] = owner
				for _, metric := range metrics {
					baselineSamples[metric] = append(baselineSamples[metric], baseline.Metrics[metric])
					productionSamples[metric] = append(productionSamples[metric], production.Metrics[metric])
				}
			}
			for _, metric := range metrics {
				upper, err := PairedUpperBound(baselineSamples[metric], productionSamples[metric])
				if err != nil {
					return ExperimentResult{}, fmt.Errorf("evaluator oracle: %s/%d/%s: %w", workload.ID, population, metric, err)
				}
				gate := Gate{WorkloadID: workload.ID, Population: population, Metric: metric, UCB: upper, Pass: upper <= UCBLimit}
				result.Gates = append(result.Gates, gate)
				result.Pass = result.Pass && gate.Pass
			}
		}
	}
	if len(result.Gates) != 171 {
		return ExperimentResult{}, fmt.Errorf("evaluator oracle: generated %d gates, want 171", len(result.Gates))
	}
	return result, nil
}

func validateMetrics(values map[Metric]float64) error {
	if len(values) != len(metrics) {
		return fmt.Errorf("got %d metrics, want %d", len(values), len(metrics))
	}
	for _, metric := range metrics {
		if !finitePositive(values[metric]) {
			return fmt.Errorf("metric %s is missing, non-positive, or non-finite", metric)
		}
	}
	return nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}
