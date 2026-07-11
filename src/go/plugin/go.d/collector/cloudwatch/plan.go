// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

const (
	maxCredentialSources = 64
	maxTargets           = 256
	maxRules             = 256
	maxReferencesPerRule = 256
	maxCompiledScopes    = 4096
)

var (
	configNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	configRegionPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)+-[0-9]+$`)
	accountIDPattern    = regexp.MustCompile(`^[0-9]{12}$`)
)

func validateConfigStructure(cfg Config) error {
	var errs []error

	if len(cfg.Credentials) == 0 {
		errs = append(errs, errors.New("'credentials' must contain at least one entry"))
	}
	if len(cfg.Credentials) > maxCredentialSources {
		errs = append(errs, fmt.Errorf("'credentials' contains %d entries; maximum is %d", len(cfg.Credentials), maxCredentialSources))
	}

	credentialNames := make([]string, 0, len(cfg.Credentials))
	for name := range cfg.Credentials {
		credentialNames = append(credentialNames, name)
	}
	sort.Strings(credentialNames)
	for _, name := range credentialNames {
		if !configNamePattern.MatchString(name) {
			errs = append(errs, fmt.Errorf("credential name %q must match %q", name, configNamePattern.String()))
		}
		if err := cfg.Credentials[name].ValidateWithPath(fmt.Sprintf("credentials.%s", name)); err != nil {
			errs = append(errs, err)
		}
	}

	if len(cfg.Targets) == 0 {
		errs = append(errs, errors.New("'targets' must contain at least one entry"))
	}
	if len(cfg.Targets) > maxTargets {
		errs = append(errs, fmt.Errorf("'targets' contains %d entries; maximum is %d", len(cfg.Targets), maxTargets))
	}
	seenTargets := make(map[string]struct{}, len(cfg.Targets))
	for i, target := range cfg.Targets {
		path := fmt.Sprintf("targets[%d]", i)
		name := strings.TrimSpace(target.Name)
		if !configNamePattern.MatchString(name) {
			errs = append(errs, fmt.Errorf("%s.name %q must match %q", path, target.Name, configNamePattern.String()))
		} else if _, ok := seenTargets[name]; ok {
			errs = append(errs, fmt.Errorf("duplicate target name %q", name))
		} else {
			seenTargets[name] = struct{}{}
		}
		credentialRef := strings.TrimSpace(target.Credentials)
		if credentialRef == "" {
			errs = append(errs, fmt.Errorf("%s.credentials is required", path))
		} else if _, ok := cfg.Credentials[credentialRef]; !ok {
			errs = append(errs, fmt.Errorf("%s.credentials references unknown credential %q", path, credentialRef))
		}
		if target.AssumeRole != nil && strings.TrimSpace(target.AssumeRole.RoleARN) == "" {
			errs = append(errs, fmt.Errorf("%s.assume_role.role_arn is required", path))
		} else if target.AssumeRole != nil {
			if _, err := rolePartition(target.AssumeRole.RoleARN); err != nil {
				errs = append(errs, fmt.Errorf("%s.assume_role.role_arn is invalid: %w", path, err))
			}
		}
	}

	if len(cfg.Rules) == 0 {
		errs = append(errs, errors.New("'rules' must contain at least one entry"))
	}
	if len(cfg.Rules) > maxRules {
		errs = append(errs, fmt.Errorf("'rules' contains %d entries; maximum is %d", len(cfg.Rules), maxRules))
	}
	seenRules := make(map[string]struct{}, len(cfg.Rules))
	for i, rule := range cfg.Rules {
		path := fmt.Sprintf("rules[%d]", i)
		name := strings.TrimSpace(rule.Name)
		if !configNamePattern.MatchString(name) {
			errs = append(errs, fmt.Errorf("%s.name %q must match %q", path, rule.Name, configNamePattern.String()))
		} else if _, ok := seenRules[name]; ok {
			errs = append(errs, fmt.Errorf("duplicate rule name %q", name))
		} else {
			seenRules[name] = struct{}{}
		}
		if len(rule.Targets) == 0 {
			errs = append(errs, fmt.Errorf("%s.targets must contain at least one target", path))
		}
		if len(rule.Targets) > maxReferencesPerRule {
			errs = append(errs, fmt.Errorf("%s.targets contains %d entries; maximum is %d", path, len(rule.Targets), maxReferencesPerRule))
		}
		if len(rule.Regions) > maxReferencesPerRule {
			errs = append(errs, fmt.Errorf("%s.regions contains %d entries; maximum is %d", path, len(rule.Regions), maxReferencesPerRule))
		}
		if err := validateRuleRegions(path, rule.Regions); err != nil {
			errs = append(errs, err)
		}
		if rule.Profiles != nil {
			if len(rule.Profiles.Include) > maxReferencesPerRule {
				errs = append(errs, fmt.Errorf("%s.profiles.include contains %d entries; maximum is %d", path, len(rule.Profiles.Include), maxReferencesPerRule))
			}
			if len(rule.Profiles.Exclude) > maxReferencesPerRule {
				errs = append(errs, fmt.Errorf("%s.profiles.exclude contains %d entries; maximum is %d", path, len(rule.Profiles.Exclude), maxReferencesPerRule))
			}
		}
		if rule.Filters != nil || rule.Labels != nil || rule.Series != nil || rule.Query != nil {
			errs = append(errs, fmt.Errorf("%s contains fields reserved for a later phase", path))
		}
	}

	return errors.Join(errs...)
}

func validateRuleRegions(path string, regions []string) error {
	if len(regions) == 0 {
		return fmt.Errorf("%s.regions must contain at least one region", path)
	}
	seen := make(map[string]struct{}, len(regions))
	var errs []error
	for i, raw := range regions {
		region := strings.ToLower(strings.TrimSpace(raw))
		if !configRegionPattern.MatchString(region) {
			errs = append(errs, fmt.Errorf("%s.regions[%d] %q is not a valid AWS region", path, i, raw))
			continue
		}
		if _, ok := seen[region]; ok {
			errs = append(errs, fmt.Errorf("%s.regions contains duplicate region %q", path, region))
			continue
		}
		seen[region] = struct{}{}
	}
	return errors.Join(errs...)
}

func compileConfig(cfg Config, catalog cwprofiles.Catalog) (*collectorRuntime, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	runtime := &collectorRuntime{
		Targets:      make([]*targetRuntime, 0, len(cfg.Targets)),
		TargetsByRef: make(map[string]*targetRuntime, len(cfg.Targets)),
	}
	usedCredentials := make(map[string]struct{})
	for _, target := range cfg.Targets {
		name := strings.TrimSpace(target.Name)
		credentialRef := strings.TrimSpace(target.Credentials)
		tr := &targetRuntime{
			Name:     name,
			Identity: awsauth.NewIdentity(name, cfg.Credentials[credentialRef], target.AssumeRole),
		}
		if target.AssumeRole != nil {
			tr.RoleARN = strings.TrimSpace(target.AssumeRole.RoleARN)
		}
		runtime.Targets = append(runtime.Targets, tr)
		runtime.TargetsByRef[name] = tr
		usedCredentials[credentialRef] = struct{}{}
	}

	allProfiles := catalog.AllProfiles()
	profilesByName := make(map[string]cwprofiles.ResolvedProfile, len(allProfiles))
	for _, profile := range allProfiles {
		profilesByName[profile.Name] = profile
	}

	usedTargets := make(map[string]struct{})
	selectedProfiles := make(map[string]cwprofiles.ResolvedProfile)
	seenScopes := make(map[string]string)
	targetPartitions := make(map[string]map[string]struct{})

	for ruleIndex, rule := range cfg.Rules {
		path := fmt.Sprintf("rules[%d]", ruleIndex)
		ruleName := strings.TrimSpace(rule.Name)
		targets, err := resolveRuleTargets(path, rule.Targets, runtime.TargetsByRef)
		if err != nil {
			return nil, err
		}
		profiles, explicitlyIncluded, err := resolveRuleProfiles(path, rule.Profiles, allProfiles, profilesByName)
		if err != nil {
			return nil, err
		}
		regions := normalizeRegions(rule.Regions)
		eligibleScopes := 0

		for _, target := range targets {
			usedTargets[target.Name] = struct{}{}
			for _, profile := range profiles {
				effective := make([]string, 0, len(regions))
				for _, region := range regions {
					if profile.Config.SupportsRegion(region) {
						effective = append(effective, region)
					}
				}
				if len(effective) == 0 {
					if _, explicit := explicitlyIncluded[profile.Name]; explicit {
						return nil, fmt.Errorf("%s explicitly includes profile %q, but none of regions %v are supported", path, profile.Name, regions)
					}
					runtime.Diagnostics = append(runtime.Diagnostics,
						fmt.Sprintf("rule %q skips default profile %q because none of regions %v are supported", ruleName, profile.Name, regions))
					continue
				}

				for _, region := range effective {
					eligibleScopes++
					key := target.Name + "\x00" + profile.Name + "\x00" + region
					if owner, ok := seenScopes[key]; ok {
						runtime.Diagnostics = append(runtime.Diagnostics,
							fmt.Sprintf("rule %q is shadowed by earlier rule %q for target %q, profile %q, region %q", ruleName, owner, target.Name, profile.Name, region))
						continue
					}
					if len(runtime.Scopes) == maxCompiledScopes {
						return nil, fmt.Errorf("compiled collection scopes exceed maximum %d", maxCompiledScopes)
					}
					seenScopes[key] = ruleName
					runtime.Scopes = append(runtime.Scopes, collectionScope{RuleName: ruleName, Target: target, Profile: profile, Region: region})
					selectedProfiles[profile.Name] = profile
					if !slices.Contains(target.Regions, region) {
						target.Regions = append(target.Regions, region)
					}
					if targetPartitions[target.Name] == nil {
						targetPartitions[target.Name] = make(map[string]struct{})
					}
					targetPartitions[target.Name][regionPartition(region)] = struct{}{}
				}
			}
		}
		if eligibleScopes == 0 {
			return nil, fmt.Errorf("%s compiles to no collection scopes", path)
		}
	}

	for _, target := range runtime.Targets {
		if _, ok := usedTargets[target.Name]; !ok {
			return nil, fmt.Errorf("target %q is not referenced by any rule", target.Name)
		}
		if len(targetPartitions[target.Name]) > 1 {
			return nil, fmt.Errorf("target %q spans multiple AWS partitions across regions %v", target.Name, target.Regions)
		}
		if err := validateRolePartition(target, targetPartitions[target.Name]); err != nil {
			return nil, err
		}
	}
	credentialNames := make([]string, 0, len(cfg.Credentials))
	for name := range cfg.Credentials {
		credentialNames = append(credentialNames, name)
	}
	sort.Strings(credentialNames)
	for _, name := range credentialNames {
		if _, ok := usedCredentials[name]; !ok {
			return nil, fmt.Errorf("credential %q is not referenced by any target", name)
		}
	}

	for _, profile := range allProfiles {
		if selected, ok := selectedProfiles[profile.Name]; ok {
			runtime.Profiles = append(runtime.Profiles, selected)
		}
	}
	runtime.Diagnostics = uniqueStrings(runtime.Diagnostics)
	return runtime, nil
}

func resolveRuleTargets(path string, refs []string, targets map[string]*targetRuntime) ([]*targetRuntime, error) {
	seen := make(map[string]struct{}, len(refs))
	out := make([]*targetRuntime, 0, len(refs))
	for i, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			return nil, fmt.Errorf("%s.targets[%d] must not be empty", path, i)
		}
		if _, ok := seen[ref]; ok {
			return nil, fmt.Errorf("%s.targets contains duplicate target %q", path, ref)
		}
		target, ok := targets[ref]
		if !ok {
			return nil, fmt.Errorf("%s.targets references unknown target %q", path, ref)
		}
		seen[ref] = struct{}{}
		out = append(out, target)
	}
	return out, nil
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
		name := strings.TrimSpace(raw)
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

func validateRolePartition(target *targetRuntime, partitions map[string]struct{}) error {
	if target.RoleARN == "" || len(partitions) == 0 {
		return nil
	}
	rolePartition, err := rolePartition(target.RoleARN)
	if err != nil {
		return fmt.Errorf("target %q has invalid role ARN: %w", target.Name, err)
	}
	for regionPartition := range partitions {
		if rolePartition != regionPartition {
			return fmt.Errorf("target %q role partition %q does not match selected region partition %q", target.Name, rolePartition, regionPartition)
		}
	}
	return nil
}

func rolePartition(roleARN string) (string, error) {
	parsed, err := arn.Parse(strings.TrimSpace(roleARN))
	if err != nil {
		return "", err
	}
	roleName := strings.TrimPrefix(parsed.Resource, "role/")
	if parsed.Service != "iam" || parsed.Region != "" || !accountIDPattern.MatchString(parsed.AccountID) ||
		roleName == parsed.Resource || roleName == "" || strings.HasPrefix(roleName, "/") || strings.HasSuffix(roleName, "/") {
		return "", fmt.Errorf("expected an IAM role ARN")
	}
	return parsed.Partition, nil
}

func (c *Collector) ensureRuntime() error {
	if c.runtime != nil {
		return nil
	}
	catalog, err := c.loadCatalog()
	if err != nil {
		return fmt.Errorf("load CloudWatch profiles: %w", err)
	}
	runtime, err := compileConfig(c.Config, catalog)
	if err != nil {
		return fmt.Errorf("compile CloudWatch collection plan: %w", err)
	}
	tpl, err := buildChartTemplate(runtime.Profiles)
	if err != nil {
		return fmt.Errorf("build CloudWatch chart template: %w", err)
	}
	for _, diagnostic := range runtime.Diagnostics {
		c.Warningf("CloudWatch collection plan: %s", diagnostic)
	}
	c.runtime = runtime
	c.invalidateQueryPlan()
	c.chartTemplateYAML = tpl
	c.Infof("CloudWatch: compiled %d collection scope(s) across %d target(s) and %d profile(s)",
		len(runtime.Scopes), len(runtime.Targets), len(runtime.Profiles))
	c.Debugf("CloudWatch tuning: update_every=%ds, discovery.refresh_every=%ds, query_offset=%ds, recently_active_only=%v",
		c.UpdateEvery, c.Discovery.RefreshEvery, c.QueryOffset, c.recentlyActiveOnly())
	return nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := values[:0]
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
