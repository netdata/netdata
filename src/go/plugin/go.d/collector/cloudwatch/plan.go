// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
)

const (
	maxCompiledScopes       = 4096
	maxCandidateScopeChecks = 16384
)

var accountIDPattern = regexp.MustCompile(`^[0-9]{12}$`)

func compileConfig(cfg Config, catalog cwprofiles.Catalog) (*collectionPlan, []string, error) {
	if err := cfg.validate(); err != nil {
		return nil, nil, err
	}
	return newPlanCompiler(cfg, catalog).compile()
}

func resolveRuleTargets(refs []string, targets map[string]*collectionTarget) []*collectionTarget {
	out := make([]*collectionTarget, 0, len(refs))
	for _, ref := range refs {
		out = append(out, targets[ref])
	}
	return out
}

func resolveRuleProfiles(path string, selector *ProfileSelectorConfig, all []cwprofiles.ResolvedProfile, byName map[string]cwprofiles.ResolvedProfile) ([]cwprofiles.ResolvedProfile, map[string]struct{}, error) {
	include, err := normalizedUniqueProfileNames(path+".profiles.include", nilIfSelector(selector, func(s *ProfileSelectorConfig) []string { return s.Include }), byName)
	if err != nil {
		return nil, nil, err
	}
	exclude, err := normalizedUniqueProfileNames(path+".profiles.exclude", nilIfSelector(selector, func(s *ProfileSelectorConfig) []string { return s.Exclude }), byName)
	if err != nil {
		return nil, nil, err
	}
	for name := range include {
		if _, ok := exclude[name]; ok {
			return nil, nil, fmt.Errorf("%s.profiles includes and excludes profile %q", path, name)
		}
	}
	if !selector.includesDefaults() && len(include) == 0 {
		return nil, nil, fmt.Errorf("%s.profiles.include must not be empty when defaults is false", path)
	}

	selected := make(map[string]struct{})
	if selector.includesDefaults() {
		for _, profile := range all {
			if !profile.Config.Disabled {
				selected[profile.Name] = struct{}{}
			}
		}
	}
	for name := range include {
		selected[name] = struct{}{}
	}
	for name := range exclude {
		delete(selected, name)
	}
	if len(selected) == 0 {
		return nil, nil, fmt.Errorf("%s.profiles selects no profiles", path)
	}

	out := make([]cwprofiles.ResolvedProfile, 0, len(selected))
	for _, profile := range all {
		if _, ok := selected[profile.Name]; ok {
			out = append(out, profile)
		}
	}
	return out, include, nil
}

func nilIfSelector(selector *ProfileSelectorConfig, fn func(*ProfileSelectorConfig) []string) []string {
	if selector == nil {
		return nil
	}
	return fn(selector)
}

func normalizedUniqueProfileNames(path string, values []string, known map[string]cwprofiles.ResolvedProfile) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(values))
	for i, raw := range values {
		name := raw
		if err := validateCanonicalString(fmt.Sprintf("%s[%d]", path, i), name); err != nil {
			return nil, err
		}
		if name == "" {
			return nil, fmt.Errorf("%s[%d] must not be empty", path, i)
		}
		if _, ok := known[name]; !ok {
			return nil, fmt.Errorf("%s references unknown profile %q", path, name)
		}
		if _, ok := out[name]; ok {
			return nil, fmt.Errorf("%s contains duplicate profile %q", path, name)
		}
		out[name] = struct{}{}
	}
	return out, nil
}

func compileProfileSeries(profile cwprofiles.ResolvedProfile) []profileSeriesSpec {
	var series []profileSeriesSpec
	for metricIndex, metric := range profile.Config.Metrics {
		for _, statistic := range metric.Statistics {
			token := cwprofiles.NormalizeStatistic(statistic)
			series = append(series, profileSeriesSpec{
				Ordinal: len(series), MetricIndex: metricIndex, Statistic: token,
				Name: cwprofiles.ExportedSeriesName(profile.Name, metric.ID, token),
			})
		}
	}
	return series
}

type profileSeriesCatalog struct {
	series            []profileSeriesSpec
	metricByName      map[string]int
	seriesByMetric    [][]int
	seriesByStatistic []map[string]int
}

type resolvedSeriesSelection struct {
	series   selectedSeriesSpec
	selected bool
}

func indexProfileSeries(profile cwprofiles.ResolvedProfile) profileSeriesCatalog {
	series := compileProfileSeries(profile)
	catalog := profileSeriesCatalog{
		series:            series,
		metricByName:      make(map[string]int, len(profile.Config.Metrics)),
		seriesByMetric:    make([][]int, len(profile.Config.Metrics)),
		seriesByStatistic: make([]map[string]int, len(profile.Config.Metrics)),
	}
	for i, metric := range profile.Config.Metrics {
		catalog.metricByName[metric.MetricName] = i
		catalog.seriesByStatistic[i] = make(map[string]int, len(metric.Statistics))
	}
	for ordinal, item := range series {
		catalog.seriesByMetric[item.MetricIndex] = append(catalog.seriesByMetric[item.MetricIndex], ordinal)
		catalog.seriesByStatistic[item.MetricIndex][item.Statistic] = ordinal
	}
	return catalog
}

func resolveRuleMetrics(path string, selectors []ProfileMetricSelectorConfig, profiles []cwprofiles.ResolvedProfile, seriesByProfile map[string]profileSeriesCatalog) (map[string][]selectedSeriesSpec, map[string]struct{}, error) {
	selected := make(map[string][]selectedSeriesSpec, len(profiles))
	explicitProfiles := make(map[string]struct{})
	profilesByName := make(map[string]cwprofiles.ResolvedProfile, len(profiles))
	seriesSelections := make(map[string][]resolvedSeriesSelection, len(profiles))
	for _, profile := range profiles {
		profilesByName[profile.Name] = profile
		catalog := seriesByProfile[profile.Name]
		selections := make([]resolvedSeriesSelection, len(catalog.series))
		for i, series := range catalog.series {
			selections[i] = resolvedSeriesSelection{
				series:   selectedSeriesSpec{profileSeriesSpec: series},
				selected: !profile.Config.Metrics[series.MetricIndex].Disabled,
			}
		}
		seriesSelections[profile.Name] = selections
	}

	explicitSelections := 0
	for i, selector := range selectors {
		groupPath := fmt.Sprintf("%s.metrics[%d]", path, i)
		profile, ok := profilesByName[selector.Profile]
		if !ok {
			return nil, nil, fmt.Errorf("%s.profile references profile %q not selected by this rule", groupPath, selector.Profile)
		}
		explicitProfiles[profile.Name] = struct{}{}
		selections := seriesSelections[profile.Name]
		if !selector.includesDefaults() {
			for i := range selections {
				selections[i] = resolvedSeriesSelection{series: selectedSeriesSpec{profileSeriesSpec: selections[i].series.profileSeriesSpec}}
			}
		}
		count, err := resolveMetricGroup(groupPath, selector, profile, seriesByProfile[profile.Name], selections)
		if err != nil {
			return nil, nil, err
		}
		explicitSelections += count
		if explicitSelections > maxReferencesPerRule {
			return nil, nil, fmt.Errorf("%s.metrics expands to more than %d metric/statistic selections", path, maxReferencesPerRule)
		}
	}

	for _, profile := range profiles {
		for _, selection := range seriesSelections[profile.Name] {
			if selection.selected {
				selected[profile.Name] = append(selected[profile.Name], selection.series)
			}
		}
	}
	return selected, explicitProfiles, nil
}

func resolveMetricGroup(path string, selector ProfileMetricSelectorConfig, profile cwprofiles.ResolvedProfile, catalog profileSeriesCatalog, selected []resolvedSeriesSelection) (int, error) {
	expanded := 0
	explicitOwners := make([]string, len(catalog.series))
	var overlapErrs []error
	for i, entry := range selector.Include {
		metricPath := fmt.Sprintf("%s.include[%d]", path, i)
		metricIndex, ok := catalog.metricByName[entry.Name]
		if !ok {
			return 0, fmt.Errorf("%s.name references unknown MetricName %q in profile %q", metricPath, entry.Name, profile.Name)
		}
		statistics := entry.Statistics
		statisticsPath := metricPath + ".statistics"
		if statistics == nil {
			statistics = selector.Statistics
			statisticsPath = path + ".statistics"
		}
		var expandedOrdinals []int
		if statistics == nil {
			expandedOrdinals = catalog.seriesByMetric[metricIndex]
		} else {
			expandedOrdinals = make([]int, 0, len(statistics))
			for j, raw := range statistics {
				statistic := normalizeMetricStatistic(raw)
				ordinal, ok := catalog.seriesByStatistic[metricIndex][statistic]
				if !ok {
					return 0, fmt.Errorf("%s[%d] %q is not exported for MetricName %q in profile %q", statisticsPath, j, raw, entry.Name, profile.Name)
				}
				expandedOrdinals = append(expandedOrdinals, ordinal)
			}
		}
		for _, ordinal := range expandedOrdinals {
			series := catalog.series[ordinal]
			if owner := explicitOwners[ordinal]; owner != "" {
				overlapErrs = append(overlapErrs, fmt.Errorf("%s selects MetricName %q statistic %q already selected by %s", metricPath, entry.Name, cwprofiles.StatString(series.Statistic), owner))
				continue
			}
			explicitOwners[ordinal] = metricPath
			selected[ordinal] = resolvedSeriesSelection{
				selected: true,
				series: selectedSeriesSpec{
					profileSeriesSpec: series,
					groupQuery:        cwquery.Source{Config: selector.Query, Path: path + ".query"},
					itemQuery:         cwquery.Source{Config: entry.Query, Path: metricPath + ".query"},
				},
			}
			expanded++
		}
	}
	return expanded, errors.Join(overlapErrs...)
}

func normalizeMetricStatistic(raw string) string {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return ""
	}
	if strings.EqualFold(raw, "SampleCount") {
		return "sample_count"
	}
	token := cwprofiles.NormalizeStatistic(raw)
	if token == "sample_count" {
		return ""
	}
	return token
}

func validateRolePartition(targetName, roleARN string, partitions map[string]struct{}) error {
	if roleARN == "" || len(partitions) == 0 {
		return nil
	}
	rolePartition, err := rolePartition(roleARN)
	if err != nil {
		return fmt.Errorf("target %q has invalid role ARN: %w", targetName, err)
	}
	for regionPartition := range partitions {
		if rolePartition != regionPartition {
			return fmt.Errorf("target %q role partition %q does not match selected region partition %q", targetName, rolePartition, regionPartition)
		}
	}
	return nil
}

func rolePartition(roleARN string) (string, error) {
	parsed, err := arn.Parse(roleARN)
	if err != nil {
		return "", fmt.Errorf("invalid ARN syntax")
	}
	roleName := strings.TrimPrefix(parsed.Resource, "role/")
	if parsed.Service != "iam" || parsed.Region != "" || !accountIDPattern.MatchString(parsed.AccountID) ||
		roleName == parsed.Resource || roleName == "" || strings.HasPrefix(roleName, "/") || strings.HasSuffix(roleName, "/") {
		return "", fmt.Errorf("expected an IAM role ARN")
	}
	return parsed.Partition, nil
}
