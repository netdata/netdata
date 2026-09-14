// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

const validationTimeout = 30 * time.Second

// Validate runs the candidate through the production parser, collector,
// chartengine, and chart emitter plus contributor-policy checks.
func Validate(ctx context.Context, opts Options) Report {
	reports, _ := validateSequence(ctx, opts, []string{opts.DumpPath}, validationMode{})
	return reports[0]
}

type validationSession struct {
	opts                 Options
	mode                 validationMode
	policy               jobPolicy
	staged               stagedValidationInputs
	profile              promprofiles.Profile
	coll                 *promcollector.Collector
	job                  effectiveJobReport
	pipeline             *pipelineDiagnosticSummary
	selectedProfiles     []promprofiles.Profile
	selectedProfileNames []string
	engine               *chartengine.Engine
	merged               *charttpl.Spec
	refs                 []chartRef
	templateYAML         string
	routes               *planRouteSummary
	retainedChartLabels  map[string][]promreplay.SemanticLabel
	initialized          bool
}

func validateSequence(parent context.Context, opts Options, dumpPaths []string, mode validationMode) ([]Report, error) {
	fail := func(code, path, message, remediation string) ([]Report, error) {
		return []Report{validationFailure(code, path, message, remediation)}, nil
	}
	if parent == nil {
		return fail("validation_context", "", "validation context is nil", "Validation requires a cancelable context.")
	}
	if err := validateInputFile(opts.ProfilePath); err != nil {
		return fail("profile_input", opts.ProfilePath, err.Error(), "The validator must read one regular profile file.")
	}
	for _, path := range opts.SupportingProfilePaths {
		if err := validateInputFile(path); err != nil {
			return fail("profile_input", path, err.Error(), "Every supporting profile must be a readable regular file explicitly supplied to the validator.")
		}
	}
	if len(dumpPaths) == 0 {
		return fail("dump_input", "", "no evidence dumps supplied", "Validation requires at least one source-complete evidence dump.")
	}
	for _, path := range dumpPaths {
		if err := validateInputFile(path); err != nil {
			return fail("dump_input", path, err.Error(), "The evidence dump must be a readable regular file.")
		}
	}
	policy, err := loadJobPolicy(opts.JobPath)
	if err != nil {
		return fail("job_policy", opts.JobPath, err.Error(), "Only the documented shaping fields are accepted, with strict YAML keys.")
	}
	staged, cleanup, err := stageValidationInputs(opts.ProfilePath, opts.SupportingProfilePaths, dumpPaths[0])
	if err != nil {
		code := "profile_load"
		path := opts.ProfilePath
		remediation := "The strict profile catalog and deferred policy decoders must accept every supplied profile."
		var inputErr *profileInputError
		if errors.As(err, &inputErr) {
			code = inputErr.code
			paths := append([]string{opts.ProfilePath}, opts.SupportingProfilePaths...)
			if inputErr.index >= 0 && inputErr.index < len(paths) {
				path = paths[inputErr.index]
			}
		}
		return fail(code, path, err.Error(), remediation)
	}
	defer cleanup()
	profile, ok := staged.catalog.Get(staged.profileName)
	if !ok {
		return fail("profile_catalog", opts.ProfilePath, "candidate missing from staged validation catalog", "Exact profile selection cannot succeed without the candidate.")
	}
	s := &validationSession{opts: opts, mode: mode, policy: policy, staged: staged, profile: profile}
	pipelineObserver := func(fact promcollector.PipelineDiagnostic) {
		if s.pipeline != nil {
			s.pipeline.observe(fact)
		}
	}
	s.coll = promcollector.NewWithOptions(
		promcollector.WithProfileCatalog(staged.catalog),
		promcollector.WithPipelineDiagnosticObserver(pipelineObserver),
	)
	s.job = applyJobPolicyMode(
		s.coll,
		policy,
		staged.fileURL,
		staged.profileNames,
		mode.automaticProfileSelection,
		mode.defaultJobName,
	)
	defer s.coll.Cleanup(context.Background())

	reports := make([]Report, 0, len(dumpPaths))
	for index, dumpPath := range dumpPaths {
		if index != 0 {
			if err := s.staged.replaceDump(dumpPath); err != nil {
				reports = append(reports, validationFailure("dump_input", dumpPath, err.Error(), "The evidence dump must remain readable throughout ordered replay."))
				if index+1 < len(dumpPaths) {
					return reports, fmt.Errorf("validation sequence step %d failed with %d fixture(s) remaining", index, len(dumpPaths)-index-1)
				}
				return reports, nil
			}
		}
		report := validateStep(parent, s, dumpPath)
		reports = append(reports, report)
		if !report.Passed() && index+1 < len(dumpPaths) {
			return reports, fmt.Errorf("validation sequence step %d failed with %d fixture(s) remaining", index, len(dumpPaths)-index-1)
		}
	}
	return reports, nil
}

func validationFailure(code, path, message, remediation string) Report {
	r := newReport()
	r.addError(code, path, message, remediation)
	sortReport(&r)
	return r
}

func validateStep(parent context.Context, s *validationSession, dumpPath string) Report {
	r := newReport()
	defer sortReport(&r)
	opts := s.opts
	mode := s.mode
	policy := s.policy
	staged := &s.staged
	profile := s.profile
	if opts.JobPath == "" {
		r.addWarning(
			"default_validation_job",
			"",
			"no structured job policy was supplied; collector defaults are being validated",
			"A deliverable profile must be tested with the same selector, relabeling, fallback types, application identity, and series limits intended for deployment.",
		)
	}

	r.Profiles.Candidate = newProfileReport(profile)
	for _, support := range staged.profiles[1:] {
		r.Profiles.Supports = append(r.Profiles.Supports, newProfileReport(support))
	}
	profileContexts := newProfileValidationContexts(*staged, &r)
	type authoredProfile struct {
		root charttpl.Group
		path string
	}
	authoredProfiles := make([]authoredProfile, 0, len(staged.profiles))
	for index, selectedProfile := range staged.profiles {
		rootPath := "template"
		matchPath := "profile.match"
		if len(staged.profiles) > 1 {
			rootPath = fmt.Sprintf("profiles[%s].template", selectedProfile.Name)
			matchPath = fmt.Sprintf("profiles[%s].match", selectedProfile.Name)
		}
		addProfileMatchHeuristics(selectedProfile.Match, matchPath, &r)
		root, err := selectedProfile.Template()
		if err != nil {
			path := opts.ProfilePath
			if index > 0 {
				path = opts.SupportingProfilePaths[index-1]
			}
			r.addError("profile_template", path, err.Error(), "Every authored profile must decode before priority, presentation, and source-intent checks run ahead of collector merge/defaulting.")
			return r
		}
		r.Counts.AuthoredCharts += inspectAuthoredCharts(root, rootPath, &r)
		authoredProfiles = append(authoredProfiles, authoredProfile{root: root, path: rootPath})
	}

	ctx, cancel := context.WithTimeout(parent, validationTimeout)
	defer cancel()

	rawSamples, err := scrapeRawSamples(ctx, staged.fileURL)
	if err != nil {
		r.addError("dump_parse", dumpPath, err.Error(), "The real Prometheus sample parser must accept the supplied exposition before job shaping.")
		return r
	}
	if duplicates := prompkg.FindSampleDuplicates(rawSamples); len(duplicates) > 0 {
		first := duplicates[0]
		sample := rawSamples.Samples[first.DuplicateIndex]
		r.addError(
			"duplicate_source_sample",
			fmt.Sprintf("samples[%d]", first.DuplicateIndex),
			fmt.Sprintf(
				"%d duplicate physical sample component(s); metric %q at classified sample %d repeats sample %d",
				len(duplicates),
				sample.Name,
				first.DuplicateIndex,
				first.FirstIndex,
			),
			"Source-complete evidence must contain each scalar or typed-family component exactly once; duplicates collapse or overwrite before objective assertions.",
		)
		return r
	}
	rawFamilies, err := prompkg.Assemble(rawSamples)
	if err != nil {
		r.addError("dump_assemble", dumpPath, err.Error(), "The production Prometheus assembler must accept the classified evidence batch.")
		return r
	}
	r.RawFamilies, r.Counts.RawLogicalSeries = inventoryRawFamilies(rawFamilies)
	r.Counts.RawFamilies = len(r.RawFamilies)
	for _, authored := range authoredProfiles {
		addAuthoredProfileHeuristics(authored.root, authored.path, r.RawFamilies, &r)
		addObservedDistributionHeuristics(authored.root, authored.path, r.RawFamilies, &r)
	}

	pipelineSummary, err := newPipelineDiagnosticSummary(policy, staged.profiles, rawSamples)
	if err != nil {
		r.addError("profile_relabeling", opts.ProfilePath, err.Error(), "The candidate profile relabeling must decode before production diagnostics can be correlated.")
		return r
	}
	s.pipeline = pipelineSummary
	coll := s.coll
	r.Job = s.job
	if !s.initialized {
		if err := coll.Init(ctx); err != nil {
			r.addError("collector_init", opts.JobPath, err.Error(), "Selector, relabeling, fallback typing, limits, and exact profile selection are validated by the real collector.")
			return r
		}
	}
	writerEligibility, err := coll.InspectWriterEligibility(rawFamilies)
	if err != nil {
		r.addError("writer_inspection", dumpPath, err.Error(), "Selector policy review must use the initialized production writer policy.")
		return r
	}
	writerEligibleFamilies := make(map[string]struct{})
	for _, family := range writerEligibility {
		if family.WritableSeries > 0 {
			writerEligibleFamilies[family.Family] = struct{}{}
		}
	}
	addJobDenyReview(policy.Selector, rawSamples, writerEligibleFamilies, &r)
	if !s.initialized {
		if err := coll.Check(ctx); err != nil {
			r.Profiles.Selected = slices.Clone(pipelineSummary.selectedProfileOrder)
			relabelAudits, ok := finalizePipelineDiagnostics(ctx, pipelineSummary, &r)
			if !ok {
				return r
			}
			addRelabelPolicyFindings(relabelAudits, &r)
			if err := addForwardCompatibilityChecks(ctx, profileContexts, policy, r.RawFamilies, rawSamples, relabelAudits,
				mode.aggregateProfileEvidence, mode.semanticCoverageReplay, &r); err != nil {
				r.addError("forward_compatibility_analysis", opts.JobPath, err.Error(), "Static matcher and relabel analysis must complete within its deterministic work budget.")
				return r
			}
			r.addError("collector_check", dumpPath, err.Error(), "The candidate must match the post-policy scrape and pass the collector startup gates.")
			return r
		}
		profilesByName := make(map[string]promprofiles.Profile, len(staged.profiles))
		for _, item := range staged.profiles {
			profilesByName[item.Name] = item
		}
		s.selectedProfileNames = slices.Clone(pipelineSummary.selectedProfileOrder)
		for _, name := range s.selectedProfileNames {
			selected, ok := profilesByName[name]
			if !ok {
				r.addError("collector_check", dumpPath, fmt.Sprintf("selected profile %q is outside the staged catalog", name), "Every automatically selected profile must be explicitly supplied as the candidate or a supporting profile.")
				return r
			}
			s.selectedProfiles = append(s.selectedProfiles, selected)
		}
		if len(s.selectedProfiles) == 0 {
			r.addError("collector_check", dumpPath, "collector selected no profile", "The candidate must match the post-policy scrape and pass the collector startup gates.")
			return r
		}
		s.initialized = true
	} else {
		pipelineSummary.selectedProfileOrder = slices.Clone(s.selectedProfileNames)
		pipelineSummary.selectedProfiles = make(map[string]struct{}, len(s.selectedProfileNames))
		for _, name := range s.selectedProfileNames {
			pipelineSummary.selectedProfiles[name] = struct{}{}
		}
	}
	r.Profiles.Selected = slices.Clone(s.selectedProfileNames)

	if err := collectAndCommit(ctx, coll); err != nil {
		var cycleErr *collectorCycleError
		errors.As(err, &cycleErr)
		switch cycleErr.stage {
		case "store":
			r.addError("collector_store", "", cycleErr.err.Error(), "Validation must drive Collect with the same begin/commit contract as the framework.")
		case "collect":
			r.addError("collector_collect", dumpPath, cycleErr.err.Error(), "The real writer pipeline must complete a collection cycle.")
		case "commit":
			r.addError("collector_commit", "", cycleErr.err.Error(), "Only successfully committed series are valid chartengine evidence.")
		}
		return r
	}
	relabelAudits, ok := finalizePipelineDiagnostics(ctx, pipelineSummary, &r)
	if !ok {
		return r
	}
	addRelabelPolicyFindings(relabelAudits, &r)
	if err := addForwardCompatibilityChecks(ctx, profileContexts, policy, r.RawFamilies, rawSamples, relabelAudits,
		mode.aggregateProfileEvidence, mode.semanticCoverageReplay, &r); err != nil {
		r.addError("forward_compatibility_analysis", opts.JobPath, err.Error(), "Static matcher and relabel analysis must complete within its deterministic work budget.")
		return r
	}

	reader := coll.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	materializedFamilies := inventoryWriterSeries(reader)
	r.Counts.WriterSeries = materializedFamilies.series
	r.PipelineExcluded, r.PipelineRenamed = reconcileRawFamilies(r.RawFamilies, r.Job, pipelineSummary)
	r.Counts.PipelineExcluded = len(r.PipelineExcluded)
	r.Counts.PipelineRenamed = len(r.PipelineRenamed)

	emitTypeID := collectorJobFullName(r.Job.Name)
	if s.engine == nil {
		s.templateYAML = coll.ChartTemplateYAML()
		if strings.TrimSpace(s.templateYAML) == "" {
			r.addError("collector_template", "", "collector returned an empty chart template", "Check must select and merge the exact candidate profile.")
			return r
		}
		s.merged, err = charttpl.DecodeYAML([]byte(s.templateYAML))
		if err != nil {
			r.addError("template_decode", "", err.Error(), "The collector's merged template must round-trip through the authoritative decoder.")
			return r
		}
		s.refs = enumerateChartRefs(s.merged)
		if err := resolveChartRefOwners(s.selectedProfiles, s.refs); err != nil {
			r.addError("template_ownership", "", err.Error(), "Every merged authored chart must resolve to exactly one selected source profile.")
			return r
		}
		routeObserver := func(fact chartengine.PlanRouteDiagnostic) {
			if s.routes != nil {
				s.routes.observe(fact)
			}
		}
		s.engine, err = newRouteEngine(s.templateYAML, emitTypeID, routeObserver)
		if err != nil {
			var planErr *routePlanError
			errors.As(err, &planErr)
			switch planErr.stage {
			case "init":
				r.addError("engine_init", "", planErr.err.Error(), "The authoritative planner must initialize.")
			case "load":
				r.addError("engine_load", "", planErr.err.Error(), "The authoritative compiler must accept the exact merged template served by the collector.")
			}
			return r
		}
	}
	merged := s.merged
	refs := s.refs
	r.AuthoredMapping = buildAuthoredMapping(refs)
	if err := addDashboardHeuristics(merged, s.selectedProfiles, refs, &r); err != nil {
		r.addError("chart_identity_policy", "", err.Error(), "The authoritative chartengine instance-label policy must accept the decoded template.")
		return r
	}

	s.routes = newPlanRouteSummary()
	planned, err := prepareRoutePlanWithEngine(reader, s.engine, s.routes)
	if err != nil {
		var planErr *routePlanError
		errors.As(err, &planErr)
		switch planErr.stage {
		case "init":
			r.addError("engine_init", "", planErr.err.Error(), "The authoritative planner must initialize.")
		case "load":
			r.addError("engine_load", "", planErr.err.Error(), "The authoritative compiler must accept the exact merged template served by the collector.")
		case "plan":
			r.addError("engine_plan", "", planErr.err.Error(), "The real chartengine must route the committed collector series.")
		case "commit":
			r.addError("engine_commit", "", planErr.err.Error(), "A successful plan must satisfy chartengine's lifecycle transaction.")
		}
		return r
	}
	plan := planned.plan
	routeSummary := planned.routes
	owners := newChartOwnershipIndex(refs, plan, routeSummary, len(s.selectedProfiles) > 1)
	if err := addObservedLabelAggregationHeuristics(merged, refs, reader, routeSummary, &r); err != nil {
		r.addError("chart_label_policy", "", err.Error(), "The authoritative chartengine instance-label policy must accept the decoded template.")
		return r
	}
	addObservedScaleHeuristics(plan, owners, &r)

	r.Charts = materializeCharts(plan, owners)
	addIncrementalUnitHeuristics(r.Charts, owners, &r)
	for _, chart := range r.Charts {
		r.Counts.ChartDimensions += len(chart.DimensionFingerprints)
		if chart.Autogen {
			r.Counts.AutogenCharts++
		} else {
			r.Counts.CuratedCharts++
		}
	}
	emitted, err := inspectEmittedPlan(plan, emitTypeID, r.Job.Name, owners)
	if err != nil {
		r.addError(
			"chart_emit",
			"",
			err.Error(),
			"The public chart emitter is the final ID/length contract before the Netdata wire protocol; a plan that cannot emit is not runtime-valid.",
		)
	} else {
		r.ChartWireCollisions = emitted.chartCollisions
		r.ContextCollisions = emitted.contextCollisions
		r.DimensionCollisions = emitted.dimensionCollisions
		if emitted.emittedCharts != emitted.plannedCharts {
			r.addError(
				"chart_wire_emission_loss",
				collisionFindingPath(emitted.unemittedChartPaths, ""),
				fmt.Sprintf(
					"chartengine planned %d new charts but the public emitter produced %d CHART commands",
					emitted.plannedCharts,
					emitted.emittedCharts,
				),
				"Every distinct planned chart must survive normalization into one public wire chart.",
			)
		}
		if len(emitted.emptyChartIDs) > 0 {
			r.addError(
				"chart_wire_id_empty",
				collisionFindingPath(emitted.emptyChartPaths, ""),
				fmt.Sprintf(
					"%d planned chart IDs normalized to an empty public wire ID (raw ID fingerprints: %s)",
					len(emitted.emptyChartIDs),
					strings.Join(emitted.emptyChartIDs, ", "),
				),
				"A chart whose ID disappears at wire normalization cannot have a stable runtime identity.",
			)
		}
		if len(emitted.chartCollisions) > 0 {
			r.addError(
				"chart_wire_id_collision_observed",
				joinedCollisionPaths(emitted.chartCollisions),
				fmt.Sprintf("%d public wire chart IDs each represent more than one planned chart", len(emitted.chartCollisions)),
				"Planner-distinct chart IDs can normalize to the same wire ID and overwrite or merge each other.",
			)
		}
		if len(emitted.emptyContexts) > 0 {
			r.addError(
				"context_wire_emission_loss",
				collisionFindingPath(emitted.emptyContextPaths, ""),
				fmt.Sprintf(
					"%d effective contexts normalized to an empty public wire value (chart ID fingerprints: %s)",
					len(emitted.emptyContexts),
					strings.Join(emitted.emptyContexts, ", "),
				),
				"A chart without its intended context cannot participate in the designed NIDL dashboard.",
			)
		}
		if len(emitted.contextCollisions) > 0 {
			r.addError(
				"context_wire_collision_observed",
				joinedContextCollisionPaths(emitted.contextCollisions),
				fmt.Sprintf("%d public wire contexts each represent multiple distinct effective contexts", len(emitted.contextCollisions)),
				"Intentional reuse of one raw context is valid, but distinct contexts must not become indistinguishable after wire normalization.",
			)
		}
		if emitted.emittedDimensions != emitted.plannedDimensions {
			r.addError(
				"dimension_wire_emission_loss",
				collisionFindingPath(emitted.unemittedDimensionPaths, ""),
				fmt.Sprintf(
					"chartengine planned %d new dimensions but the public emitter produced %d DIMENSION commands",
					emitted.plannedDimensions,
					emitted.emittedDimensions,
				),
				"A dynamic dimension name can sanitize to an empty wire ID and disappear even though chartengine materialized it.",
			)
		}
	}
	r.Counts.SeriesScanned, r.Counts.SeriesAutogen, r.Counts.SeriesUnmatched = routeSummary.counts()
	if mode.semanticFacts {
		semantics, retainedChartLabels, err := buildSemanticSnapshot(
			policy,
			s.selectedProfiles,
			rawSamples,
			reader,
			merged,
			refs,
			plan,
			emitted.inspection,
			pipelineSummary,
			routeSummary,
			s.retainedChartLabels,
		)
		if err != nil {
			r.addError(
				"semantic_facts",
				"",
				err.Error(),
				"Stock proof replay requires a complete detached mapping from raw occurrences through the production writer and authored chart routes.",
			)
		} else {
			r.semantics = semantics
			s.retainedChartLabels = retainedChartLabels
		}
	}

	routeInspection := routeSummary.inspectAuthoredCharts(refs)
	for _, item := range routeInspection.errs {
		r.addError(
			"chart_route_diagnostics",
			item.path,
			item.err.Error(),
			"The validator must correlate every authored chart with facts from the production planner.",
		)
	}
	for _, item := range routeInspection.unavailableInstances {
		r.addError(
			"instance_identity_label_unavailable",
			item.path,
			fmt.Sprintf(
				"chart %q effective instance identity requires labels %v, but selector %q matched %d writer series missing those labels",
				item.chartTitle,
				item.missingLabels,
				item.selector,
				item.series,
			),
			"Every explicit instance label must exist on every selected series. Move the chart to the correct entity boundary or override instances.by_labels with labels the series actually carries; never add a nonexistent label merely to satisfy the hierarchy.",
		)
	}
	r.DeadCharts = routeInspection.deadCharts
	r.DeadDimensions = routeInspection.deadDimensions
	r.DimensionLosses = routeInspection.dimensionLosses
	r.Collisions = routeInspection.collisions
	r.InstanceLosses = routeInspection.instanceLosses

	if r.Counts.SeriesAutogen != 0 {
		path := ""
		if paths := routeSummary.autogenProfilePaths(s.selectedProfiles); len(paths) > 0 {
			path = strings.Join(paths, ", ")
		}
		r.addError(
			"unexpected_autogen",
			path,
			fmt.Sprintf("%d series produced %d fallback charts", r.Counts.SeriesAutogen, r.Counts.AutogenCharts),
			"The source-complete fixture must curate every series that survives the collector/writer pipeline. Generic fallback preserves unknown future metrics; it does not excuse a current-source coverage gap.",
		)
	}
	if r.Counts.SeriesUnmatched != 0 {
		if paths, explained := routeSummary.unmatchedProfilePolicyPaths(s.selectedProfiles, merged); explained {
			r.addWarning(
				"profile_suppressed_series",
				strings.Join(paths, ", "),
				fmt.Sprintf("%d writer series were suppressed by selected profiles' explicit fallback selectors", r.Counts.SeriesUnmatched),
				"The samples remain in the collector store but intentionally create no fallback charts; stock profiles must bind every exact suppressed series to source-complete semantic evidence.",
			)
		} else {
			r.addError(
				"unmatched_series",
				"",
				fmt.Sprintf("%d writer series matched neither a curated chart nor autogen", r.Counts.SeriesUnmatched),
				"Unmatched series are silently absent from the dashboard and therefore violate complete post-policy coverage.",
			)
		}
	}
	if !mode.semanticCoverageReplay {
		for _, item := range r.DeadCharts {
			r.addError(
				"dead_chart",
				item.Path,
				fmt.Sprintf("chart %q did not materialize for the supplied dump", item.Title),
				"A declared chart with no evidence is either based on an invalid assumption or needs a different dump; it must not be presented as validated.",
			)
		}
		for _, item := range r.DeadDimensions {
			r.addError(
				"dead_dimension",
				item.Path,
				fmt.Sprintf("dimension selector %q did not materialize for the supplied dump", item.Selector),
				"A live sibling dimension can hide a selector or label-routing defect; every authored dimension needs observed evidence.",
			)
		}
	}
	for _, item := range r.DimensionLosses {
		r.addError(
			"dimension_materialization_loss",
			item.Path,
			fmt.Sprintf(
				"%d observed per-instance dimension identities produced only %d planned dimensions",
				item.ObservedDimensions,
				item.PlannedDimensions,
			),
			"Lifecycle caps and planner normalization must not silently discard series present in the validation evidence.",
		)
	}
	for _, item := range r.Collisions {
		r.addError(
			"rendered_id_collision",
			strings.Join(item.Charts, ", "),
			fmt.Sprintf("authored charts render the same chart ID fingerprint %q", item.RenderedIDFingerprint),
			"The full planner keeps only one owner for a rendered ID, so another chart can disappear without a schema error.",
		)
	}
	for _, item := range r.InstanceLosses {
		message := fmt.Sprintf(
			"%d observed raw instance identities produced only %d distinct rendered chart IDs",
			item.ObservedIdentities,
			item.RenderedIDs,
		)
		if item.Cause == "lifecycle_limit_or_rendered_id_collapse" {
			r.addError(
				"instance_materialization_loss_observed",
				item.Path,
				message,
				"The configured max_instances limit is below observed cardinality, or rendered IDs collapsed; either case merges or omits observed entities.",
			)
		} else {
			r.addError(
				"instance_id_collision_observed",
				item.Path,
				message,
				"Different label values can normalize to the same chart ID; the planner then sums them into one chart instead of preserving separate entities.",
			)
		}
	}
	for _, item := range r.DimensionCollisions {
		r.addError(
			"dimension_id_collision_observed",
			collisionFindingPath(item.Paths, item.ChartIDFingerprint),
			fmt.Sprintf(
				"%d authored dimensions emit the same wire ID fingerprint %q",
				item.Occurrences,
				item.DimensionIDFingerprint,
			),
			"Different dynamic dimension names can normalize to the same Netdata wire ID, causing one dimension to overwrite or merge another.",
		)
	}

	if r.Counts.WriterSeries != r.Counts.SeriesScanned {
		r.addWarning(
			"planner_series_count",
			"",
			fmt.Sprintf("writer exposed %d flattened series while chartengine scanned %d", r.Counts.WriterSeries, r.Counts.SeriesScanned),
			"An engine selector or reader-level rule may intentionally filter series, but the difference must be understood rather than treated as profile coverage.",
		)
	}
	if err := addFutureOpennessChecks(
		ctx,
		*staged,
		profileContexts,
		policy,
		rawSamples,
		coll.MetricStore().Read(metrix.ReadFlatten()),
		r.Job.Name,
		mode.automaticProfileSelection,
		mode.defaultJobName,
		&r,
	); err != nil {
		r.addError(
			"future_openness_run",
			opts.JobPath,
			err.Error(),
			"The isolated future-input collector and chart plan must complete through the same production pipeline as current evidence.",
		)
	}
	return r
}

func newProfileReport(profile promprofiles.Profile) profileReport {
	report := profileReport{Name: profile.Name, Match: profile.Match, App: profile.App}
	if selector := profile.AutogenSelector(); selector != nil {
		report.AutogenSelectorAllow = slices.Clone(selector.Allow)
		report.AutogenSelectorDeny = slices.Clone(selector.Deny)
	}
	return report
}

func finalizePipelineDiagnostics(
	ctx context.Context,
	summary *pipelineDiagnosticSummary,
	r *Report,
) (relabelPolicyAudits, bool) {
	audits, err := summary.finalize(ctx)
	if err == nil {
		return audits, true
	}
	r.addError(
		"pipeline_diagnostics",
		"",
		err.Error(),
		"Pipeline provenance and relabel attribution must finish within the validation context deadline.",
	)
	return relabelPolicyAudits{}, false
}
