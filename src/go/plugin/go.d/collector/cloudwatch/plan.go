// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
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

func compileProfileSeries(profile cwprofiles.ResolvedProfile) []compiledSeries {
	var series []compiledSeries
	for metricIndex, metric := range profile.Config.Metrics {
		period := profile.Config.EffectivePeriod(metric)
		for _, statistic := range metric.Statistics {
			token := cwprofiles.NormalizeStatistic(statistic)
			series = append(series, compiledSeries{
				Ordinal: len(series), MetricIndex: metricIndex, Statistic: token,
				Name: cwprofiles.ExportedSeriesName(profile.Name, metric.ID, token), Period: period,
			})
		}
	}
	return series
}

func resolveRuleMetrics(path string, selector *MetricSelectorConfig, profiles []cwprofiles.ResolvedProfile, seriesByProfile map[string][]compiledSeries) (map[string][]compiledSeries, map[string]struct{}, error) {
	selected := make(map[string][]compiledSeries, len(profiles))
	explicitProfiles := make(map[string]struct{})
	if selector == nil {
		for _, profile := range profiles {
			selected[profile.Name] = seriesByProfile[profile.Name]
		}
		return selected, explicitProfiles, nil
	}

	profilesByName := make(map[string]cwprofiles.ResolvedProfile, len(profiles))
	for _, profile := range profiles {
		profilesByName[profile.Name] = profile
	}
	selectedOrdinals := make(map[string]map[int]struct{})
	for i, entry := range selector.Include {
		itemPath := fmt.Sprintf("%s.metrics.include[%d]", path, i)
		profile, ok := profilesByName[entry.Profile]
		if !ok {
			return nil, nil, fmt.Errorf("%s.profile references profile %q not selected by this rule", itemPath, entry.Profile)
		}
		explicitProfiles[profile.Name] = struct{}{}

		metricIndex := -1
		for idx, metric := range profile.Config.Metrics {
			if metric.MetricName == entry.Metric {
				metricIndex = idx
				break
			}
		}
		if metricIndex < 0 {
			return nil, nil, fmt.Errorf("%s.metric references unknown MetricName %q in profile %q", itemPath, entry.Metric, profile.Name)
		}

		statistic := normalizeMetricStatistic(entry.Statistic)
		matchedOrdinal := -1
		for i := range seriesByProfile[profile.Name] {
			candidate := seriesByProfile[profile.Name][i]
			if candidate.MetricIndex == metricIndex && candidate.Statistic == statistic {
				matchedOrdinal = candidate.Ordinal
				break
			}
		}
		if matchedOrdinal < 0 {
			return nil, nil, fmt.Errorf("%s.statistic %q is not exported for MetricName %q in profile %q", itemPath, entry.Statistic, entry.Metric, profile.Name)
		}
		if selectedOrdinals[profile.Name] == nil {
			selectedOrdinals[profile.Name] = make(map[int]struct{})
		}
		selectedOrdinals[profile.Name][matchedOrdinal] = struct{}{}
	}

	for _, profile := range profiles {
		ordinals := selectedOrdinals[profile.Name]
		for _, series := range seriesByProfile[profile.Name] {
			if _, ok := ordinals[series.Ordinal]; ok {
				selected[profile.Name] = append(selected[profile.Name], series)
			}
		}
	}
	if len(explicitProfiles) == 0 {
		return nil, nil, fmt.Errorf("%s.metrics selects no metrics", path)
	}
	return selected, explicitProfiles, nil
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
