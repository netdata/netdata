// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

func addFutureOpennessChecks(
	ctx context.Context,
	isolated isolatedCatalog,
	profile promprofiles.Profile,
	policy jobPolicy,
	currentBatch prompkg.SampleBatch,
	currentReader metrix.Reader,
	jobName string,
	r *report,
) error {
	requirements, err := buildFutureRequirements(ctx, profile, policy, currentBatch)
	if err != nil {
		return fmt.Errorf("build future-input requirements: %w", err)
	}
	inputs, valid := prepareFutureInputs(
		requirements,
		policy.FutureInputs,
		currentBatch,
		profileAuthoredMetricNames(profile),
		r,
	)
	if len(inputs) > 0 {
		r.Profile.FutureMetricCanary = inputs[0].Name
	}
	if !valid {
		return nil
	}

	futureURL, err := isolated.stageFutureInputs(inputs)
	if err != nil {
		return fmt.Errorf("stage future-input evidence: %w", err)
	}
	futureBatch, err := scrapeRawSamples(ctx, futureURL)
	if err != nil {
		return fmt.Errorf("parse future-input evidence: %w", err)
	}
	if duplicates := prompkg.FindSampleDuplicates(futureBatch); len(duplicates) > 0 {
		first := duplicates[0]
		return fmt.Errorf(
			"future-input evidence contains %d duplicate physical sample component(s); classified sample %d repeats sample %d",
			len(duplicates), first.DuplicateIndex, first.FirstIndex,
		)
	}

	pipeline := newPipelineDiagnosticSummary(policy, futureBatch)
	coll := prometheus.NewWithOptions(
		prometheus.WithProfileCatalog(isolated.catalog),
		prometheus.WithPipelineDiagnosticObserver(pipeline.observe),
	)
	applyJobPolicy(coll, policy, futureURL, isolated.profileName)
	if err := coll.Init(ctx); err != nil {
		return fmt.Errorf("future collector init: %w", err)
	}
	defer coll.Cleanup(context.Background())
	if err := coll.Check(ctx); err != nil {
		return fmt.Errorf("future collector check: %w", err)
	}
	if _, selected := pipeline.selectedProfiles[profile.Name]; !selected {
		return fmt.Errorf("future collector did not select exact profile %q", profile.Name)
	}
	if err := collectAndCommit(ctx, coll); err != nil {
		return fmt.Errorf("future collector cycle: %w", err)
	}

	futureReader := coll.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	templateYAML := coll.ChartTemplateYAML()
	if strings.TrimSpace(templateYAML) == "" {
		return fmt.Errorf("future collector returned an empty chart template")
	}
	planned, err := prepareRoutePlan(futureReader, templateYAML, validationEmitTypeID(jobName))
	if err != nil {
		return fmt.Errorf("future chart plan: %w", err)
	}
	inspectFutureOpenness(
		inputs,
		requirements,
		pipeline,
		planned.routes,
		snapshotWriter(currentReader),
		futureReader,
		r,
	)
	return nil
}
