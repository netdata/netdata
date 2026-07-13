// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsregion"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
)

type planCompiler struct {
	cfg Config

	plan              *collectionPlan
	credentialsByName map[string]awsauth.CredentialConfig
	targetsByRef      map[string]*collectionTarget
	targetRoleARN     map[string]string
	profiles          []cwprofiles.ResolvedProfile
	profilesByName    map[string]cwprofiles.ResolvedProfile
	seriesByProfile   map[string][]profileSeriesSpec
	tagJoinErrors     map[string]error

	usedCredentials  map[string]struct{}
	usedTargets      map[string]struct{}
	selectedProfiles map[string]cwprofiles.ResolvedProfile
	staticOwners     map[compiledScopeKey]map[int]string
	membershipIDs    map[compiledScopeKey]int
	targetPartitions map[string]map[string]struct{}
	discoveryGroups  map[discoveryGroupKey]struct{}
	candidateChecks  int
}

type discoveryGroupKey struct {
	target, region, namespace string
}

type compiledScopeKey struct {
	target, profile, region, filter string
}

type ruleDiagnostics struct {
	ruleName              string
	skippedProfile        []string
	skippedTagAssociation []string
	shadowed              int
	shadowSample          *shadowSample
}

type shadowSample struct {
	owner, target, profile, region string
}

func newPlanCompiler(cfg Config, catalog cwprofiles.Catalog) *planCompiler {
	profiles := catalog.AllProfiles()
	profilesByName := make(map[string]cwprofiles.ResolvedProfile, len(profiles))
	seriesByProfile := make(map[string][]profileSeriesSpec, len(profiles))
	for _, profile := range profiles {
		profilesByName[profile.Name] = profile
		seriesByProfile[profile.Name] = compileProfileSeries(profile)
	}
	credentialsByName := make(map[string]awsauth.CredentialConfig, len(cfg.Credentials))
	for _, source := range cfg.Credentials {
		credentialsByName[source.Name] = source.CredentialConfig
	}
	return &planCompiler{
		cfg: cfg,
		plan: &collectionPlan{
			Targets:  make([]*collectionTarget, 0, len(cfg.Targets)),
			TagJoins: make(map[string]*tagJoin),
		},
		credentialsByName: credentialsByName,
		targetsByRef:      make(map[string]*collectionTarget, len(cfg.Targets)),
		targetRoleARN:     make(map[string]string, len(cfg.Targets)),
		profiles:          profiles,
		profilesByName:    profilesByName,
		seriesByProfile:   seriesByProfile,
		tagJoinErrors:     make(map[string]error),
		usedCredentials:   make(map[string]struct{}),
		usedTargets:       make(map[string]struct{}),
		selectedProfiles:  make(map[string]cwprofiles.ResolvedProfile),
		staticOwners:      make(map[compiledScopeKey]map[int]string),
		membershipIDs:     make(map[compiledScopeKey]int),
		targetPartitions:  make(map[string]map[string]struct{}),
		discoveryGroups:   make(map[discoveryGroupKey]struct{}),
	}
}

func (pc *planCompiler) compile() (*collectionPlan, []string, error) {
	pc.compileTargets()
	var diagnostics []string
	for i, rule := range pc.cfg.Rules {
		ruleDiagnostics, err := pc.compileRule(i, rule)
		if err != nil {
			return nil, nil, err
		}
		diagnostics = append(diagnostics, ruleDiagnostics.messages()...)
	}
	if err := pc.validateUsageAndPartitions(); err != nil {
		return nil, nil, err
	}
	pc.installSelectedProfiles()
	return pc.plan, diagnostics, nil
}

func (pc *planCompiler) compileTargets() {
	for _, target := range pc.cfg.Targets {
		name := target.Name
		credentialRef := target.Credentials
		compiled := &collectionTarget{
			Name:     name,
			Identity: awsauth.NewIdentity(name, pc.credentialsByName[credentialRef], target.AssumeRole),
		}
		pc.plan.Targets = append(pc.plan.Targets, compiled)
		pc.targetsByRef[name] = compiled
		pc.usedCredentials[credentialRef] = struct{}{}
		if target.AssumeRole != nil {
			pc.targetRoleARN[name] = target.AssumeRole.RoleARN
		}
	}
}

func (pc *planCompiler) compileRule(index int, rule RuleConfig) (ruleDiagnostics, error) {
	path := fmt.Sprintf("rules[%d]", index)
	ruleName := rule.Name
	diagnostics := ruleDiagnostics{ruleName: ruleName}

	targets := resolveRuleTargets(rule.Targets, pc.targetsByRef)
	profiles, explicitlyIncluded, err := resolveRuleProfiles(path, rule.Profiles, pc.profiles, pc.profilesByName)
	if err != nil {
		return diagnostics, err
	}
	selectedSeries, metricProfiles, err := resolveRuleMetrics(path, rule.Metrics, profiles, pc.seriesByProfile)
	if err != nil {
		return diagnostics, err
	}
	for name := range metricProfiles {
		explicitlyIncluded[name] = struct{}{}
	}
	regions := normalizeRegions(rule.Regions)
	tagFilters := compileResourceTagFilters(rule.effectiveResourceTagFilters(pc.cfg.RuleDefaults.Filters.ResourceTags))
	filterKey := resourceTagFilterSignature(tagFilters)

	selectedProfileCount := 0
	for _, profile := range profiles {
		if len(selectedSeries[profile.Name]) > 0 {
			selectedProfileCount++
		}
	}
	candidates := len(targets) * selectedProfileCount * len(regions)
	if candidates > maxCandidateScopeChecks-pc.candidateChecks {
		return diagnostics, fmt.Errorf("candidate collection scopes exceed maximum %d", maxCandidateScopeChecks)
	}
	pc.candidateChecks += candidates

	type profileRegions struct {
		profile cwprofiles.ResolvedProfile
		regions []string
		series  []compiledSeries
	}
	eligibleProfiles := make([]profileRegions, 0, len(profiles))
	for _, profile := range profiles {
		if len(selectedSeries[profile.Name]) == 0 {
			continue
		}
		var supported []string
		for _, region := range regions {
			if profile.Config.SupportsRegion(region) {
				supported = append(supported, region)
			}
		}
		if len(supported) == 0 {
			if _, explicit := explicitlyIncluded[profile.Name]; explicit {
				return diagnostics, fmt.Errorf("%s explicitly includes profile %q, but none of regions %v are supported", path, profile.Name, regions)
			}
			diagnostics.skippedProfile = append(diagnostics.skippedProfile, profile.Name)
			continue
		}
		if len(tagFilters) > 0 || len(pc.cfg.Labels.ResourceTags) > 0 {
			_, err := pc.resolveTagJoin(profile)
			if err != nil && len(tagFilters) > 0 {
				if _, explicit := explicitlyIncluded[profile.Name]; explicit {
					return diagnostics, fmt.Errorf("%s explicitly includes profile %q with resource tag filtering, but it has no safe tag association: %w", path, profile.Name, err)
				}
				diagnostics.skippedTagAssociation = append(diagnostics.skippedTagAssociation, profile.Name)
				continue
			}
		}
		series, err := resolveSeriesPolicies(path, rule.Query, pc.cfg.RuleDefaults.Query, profile, selectedSeries[profile.Name])
		if err != nil {
			return diagnostics, err
		}
		eligibleProfiles = append(eligibleProfiles, profileRegions{profile: profile, regions: supported, series: series})
	}
	if len(eligibleProfiles) == 0 {
		return diagnostics, fmt.Errorf("%s compiles to no collection scopes", path)
	}

	for _, target := range targets {
		pc.usedTargets[target.Name] = struct{}{}
		for _, selected := range eligibleProfiles {
			for _, region := range selected.regions {
				if err := pc.addScope(path, ruleName, target, selected.profile, region, tagFilters, filterKey, selected.series, &diagnostics); err != nil {
					return diagnostics, err
				}
			}
		}
	}
	return diagnostics, nil
}

func (pc *planCompiler) addScope(path, ruleName string, target *collectionTarget, profile cwprofiles.ResolvedProfile, region string, tagFilters []resourceTagFilter, filterKey string, selectedSeries []compiledSeries, diagnostics *ruleDiagnostics) error {
	key := compiledScopeKey{target: target.Name, profile: profile.Name, region: region, filter: filterKey}
	owners := pc.staticOwners[key]
	if owners == nil {
		owners = make(map[int]string, len(selectedSeries))
		pc.staticOwners[key] = owners
	}
	unshadowed := make([]compiledSeries, 0, len(selectedSeries))
	for _, series := range selectedSeries {
		if owner, ok := owners[series.Ordinal]; ok {
			diagnostics.shadowed++
			if diagnostics.shadowSample == nil {
				diagnostics.shadowSample = &shadowSample{owner: owner, target: target.Name, profile: profile.Name, region: region}
			}
			continue
		}
		owners[series.Ordinal] = ruleName
		unshadowed = append(unshadowed, series)
	}
	if len(unshadowed) == 0 {
		return nil
	}
	staticValues, static := profile.Config.StaticInstanceValues()
	if !static {
		discoveryKey := discoveryGroupKey{target: target.Name, region: region, namespace: profile.Config.Namespace}
		if _, ok := pc.discoveryGroups[discoveryKey]; !ok {
			if limit := pc.cfg.Limits.maxDiscoveryGroups(); len(pc.discoveryGroups) == limit {
				guidance := "split the collection across multiple jobs"
				if limit < maxDiscoveryGroupsPerJob {
					guidance = fmt.Sprintf("raise the safeguard up to %d for intentional scale or %s", maxDiscoveryGroupsPerJob, guidance)
				}
				return fmt.Errorf(
					"%s derives %d discovery groups (unique target, region, namespace combinations); exceeds limits.max_discovery_groups=%d; %s",
					path, len(pc.discoveryGroups)+1, limit, guidance,
				)
			}
			pc.discoveryGroups[discoveryKey] = struct{}{}
		}
	}
	if len(pc.plan.Scopes) == maxCompiledScopes {
		return fmt.Errorf("compiled collection scopes exceed maximum %d", maxCompiledScopes)
	}
	membershipID := -1
	if len(tagFilters) > 0 {
		var ok bool
		membershipID, ok = pc.membershipIDs[key]
		if !ok {
			membershipID = len(pc.membershipIDs)
			pc.membershipIDs[key] = membershipID
		}
	}
	scope := collectionScope{
		Target: target, Profile: profile, Region: region, TagFilter: tagFilters,
		TagMembershipID: membershipID, SelectedSeries: unshadowed,
	}
	if static {
		scope.StaticInstance = &discoveredInstance{DimensionValues: staticValues}
	}
	pc.plan.Scopes = append(pc.plan.Scopes, scope)
	pc.selectedProfiles[profile.Name] = profile
	if !slices.Contains(target.Regions, region) {
		target.Regions = append(target.Regions, region)
	}
	if pc.targetPartitions[target.Name] == nil {
		pc.targetPartitions[target.Name] = make(map[string]struct{})
	}
	pc.targetPartitions[target.Name][awsregion.Partition(region)] = struct{}{}
	return nil
}

func resolveSeriesPolicies(path string, rule, defaults *cwquery.Config, profile cwprofiles.ResolvedProfile, series []profileSeriesSpec) ([]compiledSeries, error) {
	out := make([]compiledSeries, len(series))
	for i, item := range series {
		metric := profile.Config.Metrics[item.MetricIndex]
		policy, err := cwquery.Resolve(path, rule, defaults, metric.Query, profile.Config.Query)
		if err != nil {
			return nil, fmt.Errorf("%s profile %q MetricName %q statistic %q: %w", path, profile.Name, metric.MetricName, item.Statistic, err)
		}
		out[i] = compiledSeries{
			Ordinal: item.Ordinal, MetricIndex: item.MetricIndex, Statistic: item.Statistic,
			Name: item.Name, Policy: policy,
		}
	}
	return out, nil
}

func (pc *planCompiler) resolveTagJoin(profile cwprofiles.ResolvedProfile) (*tagJoin, error) {
	if join, ok := pc.plan.TagJoins[profile.Name]; ok {
		return join, nil
	}
	if err, ok := pc.tagJoinErrors[profile.Name]; ok {
		return nil, err
	}
	join, err := resolveTagJoinProfile(profile)
	if err != nil {
		pc.tagJoinErrors[profile.Name] = err
		return nil, err
	}
	pc.plan.TagJoins[profile.Name] = join
	return join, nil
}

func (pc *planCompiler) validateUsageAndPartitions() error {
	for _, target := range pc.plan.Targets {
		if _, ok := pc.usedTargets[target.Name]; !ok {
			return fmt.Errorf("target %q is not referenced by any rule", target.Name)
		}
		partitions := pc.targetPartitions[target.Name]
		if len(partitions) > 1 {
			return fmt.Errorf("target %q spans multiple AWS partitions across regions %v", target.Name, target.Regions)
		}
		if err := validateRolePartition(target.Name, pc.targetRoleARN[target.Name], partitions); err != nil {
			return err
		}
	}

	for _, source := range pc.cfg.Credentials {
		if _, ok := pc.usedCredentials[source.Name]; !ok {
			return fmt.Errorf("credential %q is not referenced by any target", source.Name)
		}
	}
	return nil
}

func (pc *planCompiler) installSelectedProfiles() {
	for _, profile := range pc.profiles {
		if selected, ok := pc.selectedProfiles[profile.Name]; ok {
			pc.plan.Profiles = append(pc.plan.Profiles, selected)
		}
	}
}

func (d ruleDiagnostics) messages() []string {
	var messages []string
	if len(d.skippedProfile) > 0 {
		messages = append(messages, fmt.Sprintf("rule %q skips default profiles unsupported in its regions: %s", d.ruleName, strings.Join(d.skippedProfile, ", ")))
	}
	if len(d.skippedTagAssociation) > 0 {
		messages = append(messages, fmt.Sprintf("rule %q skips default profiles without a safe resource-tag association: %s", d.ruleName, strings.Join(d.skippedTagAssociation, ", ")))
	}
	if d.shadowed > 0 {
		sample := d.shadowSample
		messages = append(messages, fmt.Sprintf(
			"rule %q has %d metric selection(s) shadowed by earlier rules; example: rule %q owns target %q, profile %q, region %q",
			d.ruleName, d.shadowed, sample.owner, sample.target, sample.profile, sample.region,
		))
	}
	return messages
}
