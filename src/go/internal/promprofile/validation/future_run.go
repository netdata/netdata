// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
)

func addFutureOpennessChecks(
	ctx context.Context,
	staged stagedValidationInputs,
	profiles []profileValidationContext,
	policy jobPolicy,
	currentBatch prompkg.SampleBatch,
	currentReader metrix.Reader,
	jobName string,
	automaticProfiles bool,
	defaultJobName string,
	r *Report,
) error {
	subjects, err := newContributorPolicySubjects(policy, profiles)
	if err != nil {
		return fmt.Errorf("prepare future-input owners: %w", err)
	}
	owned := make([]ownedFutureRequirements, 0, len(subjects))
	for _, subject := range subjects {
		requirements, err := buildFutureRequirements(ctx, subject, currentBatch)
		if err != nil {
			return fmt.Errorf("build future-input requirements: %w", err)
		}
		owned = append(owned, ownedFutureRequirements{
			context:       subject.context,
			requirements:  requirements,
			authoredNames: subject.authoredNames,
		})
	}
	inputs, valid := prepareFutureInputs(owned, policy.FutureInputs, currentBatch, r)
	if !valid {
		return nil
	}
	requirements := combineFutureRequirements(owned)

	futureURL, err := staged.stageFutureInputs(inputs)
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

	pipeline, err := newPipelineDiagnosticSummary(policy, staged.profiles, futureBatch)
	if err != nil {
		return fmt.Errorf("prepare future pipeline diagnostics: %w", err)
	}
	coll := prometheus.NewWithOptions(
		prometheus.WithProfileCatalog(staged.catalog),
		prometheus.WithPipelineDiagnosticObserver(pipeline.observe),
	)
	applyJobPolicyMode(coll, policy, futureURL, staged.profileNames, automaticProfiles, defaultJobName)
	if err := coll.Init(ctx); err != nil {
		return fmt.Errorf("future collector init: %w", err)
	}
	defer coll.Cleanup(context.Background())
	if err := coll.Check(ctx); err != nil {
		return fmt.Errorf("future collector check: %w", err)
	}
	if !automaticProfiles {
		for _, name := range staged.profileNames {
			if _, selected := pipeline.selectedProfiles[name]; !selected {
				return fmt.Errorf("future collector did not select exact profile %q", name)
			}
		}
	}
	if err := collectAndCommit(ctx, coll); err != nil {
		return fmt.Errorf("future collector cycle: %w", err)
	}
	if _, err := pipeline.finalize(ctx); err != nil {
		return fmt.Errorf("finalize future pipeline diagnostics: %w", err)
	}

	futureReader := coll.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	templateYAML := coll.ChartTemplateYAML()
	if strings.TrimSpace(templateYAML) == "" {
		return fmt.Errorf("future collector returned an empty chart template")
	}
	planned, err := prepareRoutePlan(futureReader, templateYAML, collectorJobFullName(jobName))
	if err != nil {
		return fmt.Errorf("future chart plan: %w", err)
	}
	firstByProfile := inspectFutureOpenness(
		inputs,
		requirements,
		pipeline,
		planned.routes,
		snapshotWriter(currentReader),
		futureReader,
		r,
	)
	for index := range profiles {
		profiles[index].report.FutureRawProbe = firstByProfile[profiles[index].profile.Name]
	}
	return nil
}
