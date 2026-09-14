// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
)

func TestPipelineDiagnosticFinalizeHonorsContextCancellation(t *testing.T) {
	summary := rejectedTypedFamilySummary(t, 16)
	ctx := &cancelAfterErrChecks{remaining: 3}
	if _, err := summary.finalize(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("finalize error: got %v, want context cancellation", err)
	}
}

func TestPipelineDiagnosticConvergencePreservesEveryOriginLineage(t *testing.T) {
	summary, origins := convergedPipelineDiagnosticSummary(t, 8)
	if _, err := summary.finalize(context.Background()); err != nil {
		t.Fatal(err)
	}

	jobRule := pipelineRuleKey{
		location: pipelineRelabelLocation{stage: promcollector.PipelineRelabelStageJob},
		rule:     0,
	}
	profileRule := pipelineRuleKey{
		location: pipelineRelabelLocation{
			stage:   promcollector.PipelineRelabelStageProfile,
			profile: "candidate",
		},
		rule: 0,
	}
	for _, raw := range origins {
		if _, ok := summary.ruleForRaw(raw, jobRule); !ok {
			t.Fatalf("origin %x lost its job-stage rule", raw[:4])
		}
		if _, ok := summary.ruleForRaw(raw, profileRule); !ok {
			t.Fatalf("origin %x lost its profile-stage rule after convergence", raw[:4])
		}
		finalRaw, output, ok := summary.finalRawForOrigin(raw)
		if !ok || output.Destination.Family != "app_value" || finalRaw != output.DestinationRawIdentity {
			t.Fatalf("origin %x final output: raw=%x fact=%#v ok=%v", raw[:4], finalRaw[:4], output, ok)
		}
	}

	if got, want := summary.audits.identityCollapse.sourceIdentities, len(origins); got != want {
		t.Fatalf("collapsed source identities: got %d, want %d", got, want)
	}
	if _, ok := summary.audits.identityCollapse.locations[jobRule]; !ok {
		t.Fatalf("identity-collapse attribution lost the job-stage mutation: %#v", summary.audits.identityCollapse.locations)
	}
	if _, ok := summary.audits.identityCollapse.locations[profileRule]; !ok {
		t.Fatalf("identity-collapse attribution lost the profile-stage mutation: %#v", summary.audits.identityCollapse.locations)
	}
}

func BenchmarkPipelineDiagnosticFinalizeRejectedFamilies(b *testing.B) {
	for _, families := range []int{100, 1000} {
		b.Run(fmt.Sprintf("families_%d", families), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				b.StopTimer()
				summary := rejectedTypedFamilySummary(b, families)
				b.StartTimer()
				if _, err := summary.finalize(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPipelineDiagnosticConvergedProvenance(b *testing.B) {
	for _, samples := range []int{100, 1000} {
		b.Run(fmt.Sprintf("samples_%d", samples), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				summary, _ := convergedPipelineDiagnosticSummary(b, samples)
				if _, err := summary.finalize(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPipelineDiagnosticFinalizeUnobservedStageCount(b *testing.B) {
	for _, profiles := range []int{100, 1000} {
		b.Run(fmt.Sprintf("profiles_%d", profiles), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				b.StopTimer()
				summary, _ := convergedPipelineDiagnosticSummary(b, 100)
				for index := range profiles {
					summary.stageOrder[pipelineStageKey{
						stage:   promcollector.PipelineRelabelStageProfile,
						profile: fmt.Sprintf("unused_%06d", index),
					}] = index + 2
				}
				b.StartTimer()
				if _, err := summary.finalize(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type cancelAfterErrChecks struct {
	remaining int
}

func (c *cancelAfterErrChecks) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterErrChecks) Done() <-chan struct{}       { return nil }
func (c *cancelAfterErrChecks) Value(any) any               { return nil }
func (c *cancelAfterErrChecks) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

func rejectedTypedFamilySummary(tb testing.TB, families int) *pipelineDiagnosticSummary {
	tb.Helper()
	summary, err := newPipelineDiagnosticSummary(jobPolicy{}, nil, prompkg.SampleBatch{})
	if err != nil {
		tb.Fatal(err)
	}
	for index := range families {
		family := fmt.Sprintf("latency_%06d_seconds", index)
		rawName := fmt.Sprintf("raw_%06d_seconds", index)
		var rawIdentity prompkg.RawSampleIdentity
		rawIdentity[0] = byte(index)
		rawIdentity[1] = byte(index >> 8)
		rawIdentity[2] = byte(index >> 16)
		source := prompkg.SampleSeriesIdentity{Family: rawName}
		source.Labels[0] = byte(index)
		source.Labels[1] = byte(index >> 8)
		destination := prompkg.SampleSeriesIdentity{Family: family, Labels: source.Labels}
		destinationRaw := rawIdentity
		destinationRaw[31] = 1

		summary.observe(promcollector.PipelineDiagnostic{
			Decision:    promcollector.PipelineRawAccepted,
			RawIdentity: rawIdentity,
			Source:      source,
		})
		summary.observe(promcollector.PipelineDiagnostic{
			Decision:           promcollector.PipelineRelabelRuleEvaluated,
			RawIdentity:        rawIdentity,
			Source:             source,
			InputMetricName:    rawName,
			OutputMetricName:   family,
			RelabelRuleMatched: true,
			RelabelStage:       promcollector.PipelineRelabelStageJob,
			BlockIndex:         0,
			RuleIndex:          index,
		})
		summary.observe(promcollector.PipelineDiagnostic{
			Decision:               promcollector.PipelineRelabelOutput,
			RawIdentity:            rawIdentity,
			DestinationRawIdentity: destinationRaw,
			Source:                 source,
			Destination:            destination,
		})
		summary.observe(promcollector.PipelineDiagnostic{
			Decision:    promcollector.PipelineTypedFamilyRejected,
			Destination: destination,
			MetricName:  family,
		})
	}
	return summary
}

func convergedPipelineDiagnosticSummary(
	tb testing.TB,
	samples int,
) (*pipelineDiagnosticSummary, []prompkg.RawSampleIdentity) {
	tb.Helper()
	summary, err := newPipelineDiagnosticSummary(jobPolicy{}, nil, prompkg.SampleBatch{})
	if err != nil {
		tb.Fatal(err)
	}
	summary.stageOrder[pipelineStageKey{
		stage:   promcollector.PipelineRelabelStageProfile,
		profile: "candidate",
	}] = 1

	sharedJobSeries := prompkg.SampleSeriesIdentity{Family: "raw_value"}
	sharedProfileSeries := prompkg.SampleSeriesIdentity{Family: "app_value"}
	var sharedJobRaw, sharedProfileRaw prompkg.RawSampleIdentity
	sharedJobRaw[0] = 0xf1
	sharedProfileRaw[0] = 0xf2
	origins := make([]prompkg.RawSampleIdentity, 0, samples)

	for index := range samples {
		var raw prompkg.RawSampleIdentity
		raw[0] = byte(index)
		raw[1] = byte(index >> 8)
		raw[2] = byte(index >> 16)
		raw[31] = 1
		origins = append(origins, raw)

		source := prompkg.SampleSeriesIdentity{Family: "raw_value"}
		source.Labels[0] = byte(index)
		source.Labels[1] = byte(index >> 8)
		var value promcollector.PipelineValueIdentity
		value[0] = byte(index)
		value[1] = byte(index >> 8)
		value[2] = byte(index >> 16)
		value[31] = 1

		summary.observe(promcollector.PipelineDiagnostic{
			Decision:      promcollector.PipelineRawAccepted,
			RawIdentity:   raw,
			Source:        source,
			ValueIdentity: value,
			ScalarValue:   true,
		})
		summary.observe(promcollector.PipelineDiagnostic{
			Decision:           promcollector.PipelineRelabelRuleEvaluated,
			RawIdentity:        raw,
			Source:             source,
			ValueIdentity:      value,
			ScalarValue:        true,
			InputMetricName:    "raw_value",
			OutputMetricName:   "raw_value",
			InputLabels:        []promcollector.PipelineLabel{{Name: "source", Value: fmt.Sprintf("source-%d", index)}},
			OutputLabels:       nil,
			RelabelRuleMatched: true,
			RelabelStage:       promcollector.PipelineRelabelStageJob,
		})
		summary.observe(promcollector.PipelineDiagnostic{
			Decision:               promcollector.PipelineRelabelOutput,
			RawIdentity:            raw,
			DestinationRawIdentity: sharedJobRaw,
			Source:                 source,
			Destination:            sharedJobSeries,
			ValueIdentity:          value,
			ScalarValue:            true,
			RelabelStage:           promcollector.PipelineRelabelStageJob,
		})
		summary.observe(promcollector.PipelineDiagnostic{
			Decision:           promcollector.PipelineRelabelRuleEvaluated,
			RawIdentity:        sharedJobRaw,
			Source:             sharedJobSeries,
			ValueIdentity:      value,
			ScalarValue:        true,
			InputMetricName:    "raw_value",
			OutputMetricName:   "app_value",
			RelabelRuleMatched: true,
			RelabelStage:       promcollector.PipelineRelabelStageProfile,
			ProfileName:        "candidate",
		})
		summary.observe(promcollector.PipelineDiagnostic{
			Decision:               promcollector.PipelineRelabelOutput,
			RawIdentity:            sharedJobRaw,
			DestinationRawIdentity: sharedProfileRaw,
			Source:                 sharedJobSeries,
			Destination:            sharedProfileSeries,
			ValueIdentity:          value,
			ScalarValue:            true,
			RelabelStage:           promcollector.PipelineRelabelStageProfile,
			ProfileName:            "candidate",
		})
		summary.observe(promcollector.PipelineDiagnostic{
			Decision:      promcollector.PipelineWriterSeriesAccepted,
			Destination:   sharedProfileSeries,
			ValueIdentity: value,
			ScalarValue:   true,
		})
	}
	return summary, origins
}
