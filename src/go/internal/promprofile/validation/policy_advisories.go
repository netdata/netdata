// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	promrelabel "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

// addProfileMatchHeuristics checks whether the auto-selection signature also
// accepts common runtime/instrumentation families. Those families can be
// charted, but one generic hit is enough to select the entire profile.
func addProfileMatchHeuristics(expression, path string, r *Report) {
	m, err := matcher.NewSimplePatternsMatcher(expression)
	if err != nil {
		return // The strict profile loader reports the authoritative syntax error.
	}

	probes := []struct {
		class string
		name  string
	}{
		{class: "go_*", name: "go_memstats_alloc_bytes"},
		{class: "http_*", name: "http_requests_total"},
		{class: "process_*", name: "process_cpu_seconds_total"},
		{class: "python_*", name: "python_gc_collections_total"},
	}
	var matched []string
	for _, probe := range probes {
		if m.MatchString(probe.name) {
			matched = append(matched, probe.class)
		}
	}
	if len(matched) == 0 {
		return
	}

	r.addWarning(
		"generic_profile_match",
		path,
		fmt.Sprintf("profile detection also accepts generic family classes %v", matched),
		"Automatic selection needs only one matching scraped family. Generic runtime and instrumentation families can be charted without participating in detection; keep match exporter-specific unless broad selection is deliberate.",
	)
}

// addJobDenyReview reports observed impact, not a policy verdict. A deny can be
// correct, but its diagnostic loss must be a conscious dashboard decision.
func addJobDenyReview(
	expr promselector.Expr,
	batch prompkg.SampleBatch,
	writerEligibleFamilies map[string]struct{},
	r *Report,
) {
	if len(expr.Allow) == 0 && len(expr.Deny) == 0 {
		return
	}

	allow, err := (promselector.Expr{Allow: expr.Allow}).Parse()
	if err != nil {
		return // Collector.Init reports the authoritative selector error.
	}
	effective, err := expr.Parse()
	if err == nil && effective != nil {
		allIdentities := make(map[prompkg.SampleSeriesIdentity]struct{})
		excludedIdentities := make(map[prompkg.SampleSeriesIdentity]struct{})
		excludedFamilies := make(map[string]struct{})
		excludedRawSeries := 0
		for _, sample := range batch.Samples {
			family := prompkg.SampleFamilyName(sample)
			if _, eligible := writerEligibleFamilies[family]; !eligible {
				continue
			}
			identity := prompkg.IdentifySampleSeries(sample)
			allIdentities[identity] = struct{}{}
			if effective.Matches(sample.LabelsWithName()) {
				continue
			}
			excludedIdentities[identity] = struct{}{}
			excludedFamilies[family] = struct{}{}
			excludedRawSeries++
		}
		if len(excludedIdentities) > 0 {
			r.addWarning(
				"job_policy_exclusion_summary",
				"selector",
				fmt.Sprintf(
					"job selector removes %d of %d observed writer-capable logical identities across %d families (%d raw exposition series) before profile coverage is measured",
					len(excludedIdentities),
					len(allIdentities),
					len(excludedFamilies),
					excludedRawSeries,
				),
				"A mechanical PASS covers only the post-policy denominator. Use hierarchy and priority for dashboard focus; filtering distinct writer-capable diagnostics merely makes the gate easier by deleting evidence.",
			)
		}
	}
	if len(expr.Allow) > 0 {
		families := make(map[string]struct{})
		logicalIdentities := make(map[prompkg.SampleSeriesIdentity]struct{})
		rawSeries := 0
		for _, sample := range batch.Samples {
			family := prompkg.SampleFamilyName(sample)
			if _, eligible := writerEligibleFamilies[family]; !eligible {
				continue
			}
			if allow != nil && allow.Matches(sample.LabelsWithName()) {
				continue
			}
			families[family] = struct{}{}
			logicalIdentities[prompkg.IdentifySampleSeries(sample)] = struct{}{}
			rawSeries++
		}
		if len(logicalIdentities) > 0 {
			r.addWarning(
				"job_allow_exclusion_review",
				"selector.allow",
				fmt.Sprintf(
					"allow expressions exclude %d observed logical identities across %d otherwise writer-capable families (%d raw exposition series)",
					len(logicalIdentities),
					len(families),
					rawSeries,
				),
				"An allow list defines the dashboard's raw evidence boundary. Confirm that excluded exporter/runtime surfaces are intentionally delegated or discarded; post-policy coverage cannot recover them.",
			)
		}
	}
	for i, expression := range expr.Deny {
		deny, err := promselector.Parse(expression)
		if err != nil {
			continue // Collector.Init reports the authoritative selector error.
		}

		families := make(map[string]struct{})
		logicalIdentities := make(map[prompkg.SampleSeriesIdentity]struct{})
		rawSeries := 0
		for _, sample := range batch.Samples {
			family := prompkg.SampleFamilyName(sample)
			if _, eligible := writerEligibleFamilies[family]; !eligible {
				continue
			}
			labels := sample.LabelsWithName()
			if (allow != nil && !allow.Matches(labels)) || deny == nil || !deny.Matches(labels) {
				continue
			}
			families[family] = struct{}{}
			logicalIdentities[prompkg.IdentifySampleSeries(sample)] = struct{}{}
			rawSeries++
		}
		if len(logicalIdentities) == 0 {
			continue
		}

		r.addWarning(
			"job_deny_review",
			fmt.Sprintf("selector.deny[%d]", i),
			fmt.Sprintf(
				"deny expression matches %d observed logical identities across %d otherwise writer-capable families (%d raw exposition series)",
				len(logicalIdentities),
				len(families),
				rawSeries,
			),
			"An exclusion can be correct, but current zero/constant values or similar names do not prove redundancy. Confirm authoritative semantics and state which operator question is lost.",
		)
	}
}

type relabelDiscardAudit struct {
	action            promrelabel.Action
	blockMatch        string
	families          map[string]struct{}
	metricNames       map[string]struct{}
	logicalIdentities map[prompkg.SampleSeriesIdentity]struct{}
	rawSamples        map[prompkg.RawSampleIdentity]struct{}
	rawSeries         int
}

type relabelDiscardRuleKey struct {
	stage   promcollector.PipelineRelabelStage
	profile string
	block   int
	rule    int
}

type relabelNameRewriteAudit struct {
	metricNames          map[string]struct{}
	blockInputLabelNames map[string]struct{}
}

type relabelIdentityCollapseAudit struct {
	finalFamilies    []string
	finalIdentities  int
	sourceIdentities int
	locations        map[pipelineRuleKey]struct{}
}

type relabelTypedFamilyRejectAudit struct {
	locations map[pipelineRuleKey]struct{}
}

type relabelInvalidNameDropAudit struct {
	blocks            map[pipelineRelabelLocation]struct{}
	families          map[string]struct{}
	logicalIdentities map[prompkg.SampleSeriesIdentity]struct{}
	rawSamples        map[prompkg.RawSampleIdentity]struct{}
	rawSeries         int
}

type pipelineProvenance map[prompkg.SampleSeriesIdentity]map[pipelineDestinationOccurrence]struct{}

func (p pipelineProvenance) sourceDestinations(source prompkg.SampleSeriesIdentity) map[pipelineDestinationOccurrence]struct{} {
	destinations := p[source]
	if destinations == nil {
		destinations = make(map[pipelineDestinationOccurrence]struct{})
		p[source] = destinations
	}
	return destinations
}

type relabelPolicyAudits struct {
	discards           map[relabelDiscardRuleKey]*relabelDiscardAudit
	nameRewrites       map[relabelDiscardRuleKey]*relabelNameRewriteAudit
	blockInputs        map[pipelineRelabelLocation]map[string]struct{}
	identityCollapse   relabelIdentityCollapseAudit
	typedFamilyRejects map[string]relabelTypedFamilyRejectAudit
	invalidNameDrops   relabelInvalidNameDropAudit
	provenance         pipelineProvenance
	qualifyProfilePath bool
}

func sampleDiscardingRelabelAction(action promrelabel.Action) (promrelabel.Action, bool) {
	action = promrelabel.NormalizeAction(action)
	switch action {
	case promrelabel.Drop, promrelabel.DropEqual, promrelabel.Keep, promrelabel.KeepEqual:
		return action, true
	default:
		return "", false
	}
}
