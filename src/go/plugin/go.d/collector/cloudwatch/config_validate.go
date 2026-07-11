// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsregion"
)

const (
	maxCredentialSources = 64
	maxTargets           = 64
	maxRules             = 256
	maxReferencesPerRule = 256
)

var configNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func validateConfigStructure(cfg Config) error {
	if err := validateRawLimits(cfg); err != nil {
		return err
	}
	credentialNames, credentialErr := validateCredentials(cfg)
	return errors.Join(
		credentialErr,
		validateTargets(cfg, credentialNames),
		validateRules(cfg),
	)
}

// validateRawLimits runs before any allocation proportional to user input.
func validateRawLimits(cfg Config) error {
	switch {
	case len(cfg.Credentials) > maxCredentialSources:
		return fmt.Errorf("'credentials' contains %d entries; maximum is %d", len(cfg.Credentials), maxCredentialSources)
	case len(cfg.Targets) > maxTargets:
		return fmt.Errorf("'targets' contains %d entries; maximum is %d", len(cfg.Targets), maxTargets)
	case len(cfg.Rules) > maxRules:
		return fmt.Errorf("'rules' contains %d entries; maximum is %d", len(cfg.Rules), maxRules)
	}
	for i, rule := range cfg.Rules {
		path := fmt.Sprintf("rules[%d]", i)
		switch {
		case len(rule.Targets) > maxReferencesPerRule:
			return fmt.Errorf("%s.targets contains %d entries; maximum is %d", path, len(rule.Targets), maxReferencesPerRule)
		case len(rule.Regions) > maxReferencesPerRule:
			return fmt.Errorf("%s.regions contains %d entries; maximum is %d", path, len(rule.Regions), maxReferencesPerRule)
		case rule.Profiles != nil && len(rule.Profiles.Include) > maxReferencesPerRule:
			return fmt.Errorf("%s.profiles.include contains %d entries; maximum is %d", path, len(rule.Profiles.Include), maxReferencesPerRule)
		case rule.Profiles != nil && len(rule.Profiles.Exclude) > maxReferencesPerRule:
			return fmt.Errorf("%s.profiles.exclude contains %d entries; maximum is %d", path, len(rule.Profiles.Exclude), maxReferencesPerRule)
		}
	}
	return nil
}

func validateCredentials(cfg Config) (map[string]struct{}, error) {
	if len(cfg.Credentials) == 0 {
		return nil, errors.New("'credentials' must contain at least one entry")
	}

	names := make(map[string]struct{}, len(cfg.Credentials))
	seen := make(map[string]struct{}, len(cfg.Credentials))
	var errs []error
	for i, source := range cfg.Credentials {
		path := fmt.Sprintf("credentials[%d]", i)
		name := source.Name
		names[name] = struct{}{}
		if err := validateCanonicalString(path+".name", name); err != nil {
			errs = append(errs, err)
		} else if !configNamePattern.MatchString(name) {
			errs = append(errs, fmt.Errorf("%s.name %q must match %q", path, name, configNamePattern.String()))
		} else if _, ok := seen[name]; ok {
			errs = append(errs, fmt.Errorf("duplicate credential name %q", name))
		} else {
			seen[name] = struct{}{}
		}
		if err := source.CredentialConfig.ValidateWithPath(path); err != nil {
			errs = append(errs, err)
		}
	}
	return names, errors.Join(errs...)
}

func validateTargets(cfg Config, credentialNames map[string]struct{}) error {
	if len(cfg.Targets) == 0 {
		return errors.New("'targets' must contain at least one entry")
	}
	seen := make(map[string]struct{}, len(cfg.Targets))
	var errs []error
	for i, target := range cfg.Targets {
		path := fmt.Sprintf("targets[%d]", i)
		name := target.Name
		if err := validateCanonicalString(path+".name", name); err != nil {
			errs = append(errs, err)
		} else if !configNamePattern.MatchString(name) {
			errs = append(errs, fmt.Errorf("%s.name %q must match %q", path, target.Name, configNamePattern.String()))
		} else if _, ok := seen[name]; ok {
			errs = append(errs, fmt.Errorf("duplicate target name %q", name))
		} else {
			seen[name] = struct{}{}
		}

		credentialRef := target.Credentials
		if err := validateCanonicalString(path+".credentials", credentialRef); err != nil {
			errs = append(errs, err)
		} else if credentialRef == "" {
			errs = append(errs, fmt.Errorf("%s.credentials is required", path))
		} else if _, ok := credentialNames[credentialRef]; !ok {
			errs = append(errs, fmt.Errorf("%s.credentials references unknown credential %q", path, credentialRef))
		}
		if target.AssumeRole != nil {
			if err := validateCanonicalString(path+".assume_role.role_arn", target.AssumeRole.RoleARN); err != nil {
				errs = append(errs, err)
			} else if target.AssumeRole.RoleARN == "" {
				errs = append(errs, fmt.Errorf("%s.assume_role.role_arn is required", path))
			} else if _, err := rolePartition(target.AssumeRole.RoleARN); err != nil {
				errs = append(errs, fmt.Errorf("%s.assume_role.role_arn is invalid: %w", path, err))
			}
			if err := validateCanonicalString(path+".assume_role.external_id", target.AssumeRole.ExternalID); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func validateRules(cfg Config) error {
	if len(cfg.Rules) == 0 {
		return errors.New("'rules' must contain at least one entry")
	}
	seen := make(map[string]struct{}, len(cfg.Rules))
	var errs []error
	for i, rule := range cfg.Rules {
		path := fmt.Sprintf("rules[%d]", i)
		name := rule.Name
		if err := validateCanonicalString(path+".name", name); err != nil {
			errs = append(errs, err)
		} else if !configNamePattern.MatchString(name) {
			errs = append(errs, fmt.Errorf("%s.name %q must match %q", path, rule.Name, configNamePattern.String()))
		} else if _, ok := seen[name]; ok {
			errs = append(errs, fmt.Errorf("duplicate rule name %q", name))
		} else {
			seen[name] = struct{}{}
		}
		if len(rule.Targets) == 0 {
			errs = append(errs, fmt.Errorf("%s.targets must contain at least one target", path))
		} else {
			for j, target := range rule.Targets {
				if err := validateCanonicalString(fmt.Sprintf("%s.targets[%d]", path, j), target); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if err := validateRuleRegions(path, rule.Regions); err != nil {
			errs = append(errs, err)
		}
		for _, field := range []struct {
			name  string
			value any
		}{
			{name: "filters", value: rule.Filters},
			{name: "labels", value: rule.Labels},
			{name: "series", value: rule.Series},
			{name: "query", value: rule.Query},
		} {
			if field.value != nil {
				errs = append(errs, fmt.Errorf("%s.%s is reserved for a later phase", path, field.name))
			}
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
		region := awsregion.Normalize(raw)
		if raw != region {
			errs = append(errs, fmt.Errorf("%s.regions[%d] %q is not canonical; use %q", path, i, raw, region))
			continue
		}
		if !awsregion.Valid(region) {
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

func validateCanonicalString(path, value string) error {
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not contain surrounding whitespace", path)
	}
	return nil
}
