// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

// collect scrapes the endpoint and writes the metric families to the metrix store. The
// store cycle (begin/commit) is driven by the framework around Collect, so this only
// writes observations and returns an error to abort the cycle.
func (c *Collector) collect(ctx context.Context) error {
	runtime := c.runtime
	if runtime == nil {
		return fmt.Errorf("prometheus collector runtime is unavailable: successful Check is required before Collect")
	}

	var (
		mfs        prometheus.MetricFamilies
		typesBound bool
		err        error
	)
	if !runtime.hasProfileSamplePolicy() {
		mfs, err = c.scrape(ctx, false)
	} else {
		mfs, err = c.scrapeProfilePipeline(ctx, runtime, false)
		typesBound = true
	}
	if err != nil {
		return err
	}
	if typesBound {
		c.writer.writeBoundMetricFamilies(mfs)
	} else {
		c.writer.writeMetricFamilies(mfs)
	}
	return nil
}

// check probes the endpoint and expected prefix. Unprofiled jobs retain the startup
// source limit; selected profiles apply the existing limits after chart aggregation.
func (c *Collector) check(ctx context.Context) error {
	candidate, mfs, typesBound, err := c.checkRuntimeCandidate(ctx)
	if err != nil {
		return err
	}
	profiled := len(candidate.profiles) > 0
	if !profiled && c.MaxTS > 0 {
		if n := calcMetrics(mfs); n > c.MaxTS {
			return fmt.Errorf("'%s' num of time series (%d) > limit (%d)", c.URL, n, c.MaxTS)
		}
	}
	// Profile rollups need the full source population. Their limits apply in the planner.
	probe := *c.writer
	probe.policy.maxTSPerMetric = c.MaxTSPerMetric
	if profiled {
		probe.policy.maxTSPerMetric = 0
	}
	var writable int
	if typesBound {
		writable = probe.countBoundWritable(mfs)
	} else {
		writable = probe.countWritable(mfs)
	}
	if writable == 0 {
		return fmt.Errorf("endpoint '%s' exposes no usable metrics", c.URL)
	}
	tmpl, err := buildMergedChartTemplate(c.resolveApp(candidate.profiles), candidate.profiles)
	if err != nil {
		return err
	}
	candidate.chartTemplate = tmpl
	c.writer.policy.maxTSPerMetric = probe.policy.maxTSPerMetric
	c.runtime = candidate
	return nil
}

func (c *Collector) checkRuntimeCandidate(ctx context.Context) (*promRuntime, prometheus.MetricFamilies, bool, error) {
	candidate := &promRuntime{}
	if c.Profiles.effectiveMode() == profilesModeNone {
		mfs, err := c.scrape(ctx, true)
		if err != nil {
			return nil, nil, false, err
		}
		if err := c.validateExpectedPrefix(mfs); err != nil {
			return nil, nil, false, err
		}
		return candidate, mfs, false, nil
	}

	batch, err := c.prom.ScrapeSamples(ctx)
	if err != nil {
		return nil, nil, false, err
	}

	postJob := batch
	var postJobFamilies prometheus.MetricFamilies
	if c.jobRelabel == nil && c.pipelineObserver == nil {
		postJobFamilies, err = prometheus.Assemble(postJob)
	} else {
		postJob, postJobFamilies, err = c.relabelAndAssemble(postJob, c.jobRelabel, jobRelabelStage, true)
	}
	if err != nil {
		return nil, nil, false, err
	}
	if err := c.validateNonEmpty(postJobFamilies); err != nil {
		return nil, nil, false, err
	}
	if err := c.validateExpectedPrefix(postJobFamilies); err != nil {
		return nil, nil, false, err
	}

	candidate.profiles, err = c.selectProfiles(postJobFamilies)
	if err != nil {
		return nil, nil, false, err
	}
	for _, profile := range candidate.profiles {
		c.observePipeline(PipelineDiagnostic{
			Decision:    PipelineProfileSelected,
			ProfileName: profile.Name,
		})
	}
	normalizationOrder := profilesInNormalizationOrder(candidate.profiles, c.Profiles)
	candidate.fallbacks, err = compileProfileFallbacks(normalizationOrder)
	if err != nil {
		return nil, nil, false, err
	}
	candidate.normalizers, err = compileProfileNormalizers(normalizationOrder)
	if err != nil {
		return nil, nil, false, err
	}
	if !candidate.hasProfileSamplePolicy() {
		return candidate, postJobFamilies, false, nil
	}

	finalFamilies, err := c.applyProfilePipeline(postJob, candidate, true)
	if err != nil {
		return nil, nil, false, err
	}
	if err := c.validateNonEmpty(finalFamilies); err != nil {
		return nil, nil, false, err
	}
	return candidate, finalFamilies, true, nil
}

func (c *Collector) scrapeProfilePipeline(
	ctx context.Context,
	runtime *promRuntime,
	checking bool,
) (prometheus.MetricFamilies, error) {
	batch, err := c.prom.ScrapeSamples(ctx)
	if err != nil {
		return nil, err
	}
	if c.jobRelabel != nil || c.pipelineObserver != nil {
		batch, err = c.relabelAndValidateBatch(batch, c.jobRelabel, jobRelabelStage, checking)
		if err != nil {
			return nil, err
		}
	}
	mfs, err := c.applyProfilePipeline(batch, runtime, checking)
	if err != nil {
		return nil, err
	}
	if err := c.validateNonEmpty(mfs); err != nil {
		return nil, err
	}
	return mfs, nil
}

func (c *Collector) applyProfilePipeline(
	batch prometheus.SampleBatch,
	runtime *promRuntime,
	checking bool,
) (prometheus.MetricFamilies, error) {
	// Bind all source-name fallback decisions before profile relabeling. The
	// bound writer deliberately refuses destination-name fallback so a profile
	// rename cannot create eligibility or share it with an unbound sibling.
	c.writer.bindFallbackTypes(&batch, runtime.fallbacks)
	if len(runtime.normalizers) == 0 {
		return prometheus.Assemble(batch)
	}
	return c.profileRelabelAndAssemble(batch, runtime.normalizers, checking)
}

// scrape fetches the endpoint and enforces the empty-scrape contract: an empty scrape is
// an error (endpoint down or exposing nothing), not silent no-data. checking is true under
// Check, where relabel-corrupted typed families are a hard error rather than dropped.
func (c *Collector) scrape(ctx context.Context, checking bool) (prometheus.MetricFamilies, error) {
	mfs, err := c.scrapeMetricFamilies(ctx, checking)
	if err != nil {
		return nil, err
	}
	if err := c.validateNonEmpty(mfs); err != nil {
		return nil, err
	}
	return mfs, nil
}

func (c *Collector) validateNonEmpty(mfs prometheus.MetricFamilies) error {
	if mfs.Len() == 0 {
		return fmt.Errorf("endpoint '%s' returned 0 metric families", c.URL)
	}
	return nil
}

func (c *Collector) validateExpectedPrefix(mfs prometheus.MetricFamilies) error {
	if c.ExpectedPrefix != "" && !hasPrefix(mfs, c.ExpectedPrefix) {
		return fmt.Errorf("'%s' metrics have no expected prefix (%s)", c.URL, c.ExpectedPrefix)
	}
	return nil
}

// scrapeMetricFamilies fetches and assembles. With no relabel rules it uses the direct,
// no-buffering Scrape path (steady-state cost unchanged). With rules it scrapes the flat
// classified sample stream and runs the relabel pipeline (relabel, assemble, curate typed
// families) in relabelAndAssemble.
func (c *Collector) scrapeMetricFamilies(ctx context.Context, checking bool) (prometheus.MetricFamilies, error) {
	if c.jobRelabel == nil && c.pipelineObserver == nil {
		return c.prom.ScrapeContext(ctx)
	}

	batch, err := c.prom.ScrapeSamples(ctx)
	if err != nil {
		return nil, err
	}
	_, mfs, err := c.relabelAndAssemble(batch, c.jobRelabel, jobRelabelStage, checking)
	return mfs, err
}

func hasPrefix(mfs prometheus.MetricFamilies, prefix string) bool {
	for name := range mfs {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func calcMetrics(mfs prometheus.MetricFamilies) int {
	var n int
	for _, mf := range mfs {
		n += len(mf.Metrics())
	}
	return n
}
