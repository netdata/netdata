package oracle

import (
	"math"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
)

func TestNearestRankP99IsUntrimmed(t *testing.T) {
	samples := make([]int64, 100)
	for index := range samples {
		samples[index] = int64(100 - index)
	}
	got, err := NearestRankP99(samples)
	if err != nil {
		t.Fatal(err)
	}
	if got != 99 {
		t.Fatalf("p99=%d, want 99", got)
	}
}

func TestMetricsExposeExactNineteenGateInputs(t *testing.T) {
	want := map[Metric]struct{}{
		MetricMixedP50: {}, MetricMixedP95: {}, MetricMixedP99: {},
		MetricJobManagerP50: {}, MetricJobManagerP95: {}, MetricJobManagerP99: {},
		MetricFunctionP50: {}, MetricFunctionP95: {}, MetricFunctionP99: {},
		MetricCancelP50: {}, MetricCancelP95: {}, MetricCancelP99: {},
		MetricDeadlineP50: {}, MetricDeadlineP95: {}, MetricDeadlineP99: {},
		MetricCapacityP50: {}, MetricCapacityP95: {}, MetricCapacityP99: {},
		MetricComparativeNS: {},
	}
	got := Metrics()
	if len(got) != len(want) {
		t.Fatalf("metrics=%d, want %d", len(got), len(want))
	}
	for _, metric := range got {
		if _, exists := want[metric]; !exists {
			t.Fatalf("unexpected metric %q", metric)
		}
		delete(want, metric)
	}
	if len(want) != 0 {
		t.Fatalf("missing metrics: %#v", want)
	}
}

func TestPairedUpperBoundAcceptsAndRejectsTenPercentGate(t *testing.T) {
	baseline := constantSamples(100)
	passing, err := PairedUpperBound(baseline, constantSamples(109))
	if err != nil {
		t.Fatal(err)
	}
	failing, err := PairedUpperBound(baseline, constantSamples(111))
	if err != nil {
		t.Fatal(err)
	}
	if passing > UCBLimit || failing <= UCBLimit || math.Abs(passing-1.09) > 1e-12 || math.Abs(failing-1.11) > 1e-12 {
		t.Fatalf("upper bounds differ: passing=%v failing=%v", passing, failing)
	}
	invalid := constantSamples(100)
	invalid[7] = math.NaN()
	if _, err := PairedUpperBound(baseline, invalid); err == nil {
		t.Fatal("non-finite pair accepted")
	}
}

func TestEvaluateExperimentBuildsExactly171IndependentGates(t *testing.T) {
	summaries := syntheticExperiment(105)
	result, err := EvaluateExperiment(summaries)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pass || len(result.Gates) != 171 {
		t.Fatalf("experiment result differs: pass=%v gates=%d", result.Pass, len(result.Gates))
	}
	for _, gate := range result.Gates {
		if !gate.Pass || math.Abs(gate.UCB-1.05) > 1e-12 {
			t.Fatalf("gate differs: %#v", gate)
		}
	}
}

func TestEvaluateExperimentRejectsPairEvidenceMismatchAndClassRegression(t *testing.T) {
	summaries := syntheticExperiment(105)
	for index := range summaries {
		if summaries[index].Side == SideProduction {
			summaries[index].EqualitySHA256 = testDigest([]byte("mismatch"))
			break
		}
	}
	if _, err := EvaluateExperiment(summaries); err == nil {
		t.Fatal("mismatched pair transcript accepted")
	}

	summaries = syntheticExperiment(105)
	for index := range summaries {
		if summaries[index].Side == SideProduction {
			summaries[index].Metrics[MetricCancelP99] = 120
		}
	}
	result, err := EvaluateExperiment(summaries)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pass {
		t.Fatal("class-specific regression passed aggregate gates")
	}
	failed := 0
	for _, gate := range result.Gates {
		if !gate.Pass {
			failed++
			if gate.Metric != MetricCancelP99 {
				t.Fatalf("unexpected failed gate: %#v", gate)
			}
		}
	}
	if failed != 9 {
		t.Fatalf("got %d failed cancel gates, want 9", failed)
	}
}

func TestEvaluateExperimentRejectsNonceReuseAcrossPairKeys(t *testing.T) {
	summaries := syntheticExperiment(105)
	var firstNonce string
	for index := range summaries {
		if summaries[index].PairIndex == 0 {
			firstNonce = summaries[index].PairNonceSHA256
			break
		}
	}
	for index := range summaries {
		if summaries[index].PairIndex == 1 && summaries[index].WorkloadID == contract.PerformanceWorkloads()[0].ID && summaries[index].Population == 1 {
			summaries[index].PairNonceSHA256 = firstNonce
		}
	}
	if _, err := EvaluateExperiment(summaries); err == nil {
		t.Fatal("pair nonce reused across coordinates was accepted")
	}
}

func constantSamples(value float64) []float64 {
	samples := make([]float64, contract.PerformancePairCount)
	for index := range samples {
		samples[index] = value
	}
	return samples
}

func syntheticExperiment(productionValue float64) []RunSummary {
	summaries := make([]RunSummary, 0, len(contract.PerformanceWorkloads())*3*contract.PerformancePairCount*2)
	environment := testDigest([]byte("environment"))
	for _, workload := range contract.PerformanceWorkloads() {
		for _, population := range []int{1, 32, 256} {
			for pair := range contract.PerformancePairCount {
				baselineFirst, _ := contract.BaselineRunsFirst(pair)
				pairLabel := workload.ID + "/" + string(rune(population)) + "/" + string(rune(pair))
				nonce := testDigest([]byte("nonce/" + pairLabel))
				equality := testDigest([]byte("equality/" + pairLabel))
				for _, side := range []Side{SideBaseline, SideProduction} {
					value := 100.0
					if side == SideProduction {
						value = productionValue
					}
					values := make(map[Metric]float64, len(metrics))
					for _, metric := range metrics {
						values[metric] = value
					}
					summaries = append(summaries, RunSummary{
						WorkloadID: workload.ID, Population: population, PairIndex: pair, Side: side,
						RanFirst: baselineFirst == (side == SideBaseline), PairNonceSHA256: nonce,
						EnvironmentSHA256: environment, TranscriptSHA256: testDigest([]byte("transcript/" + pairLabel + "/" + string(side))),
						EqualitySHA256: equality, Metrics: values,
					})
				}
			}
		}
	}
	return summaries
}
