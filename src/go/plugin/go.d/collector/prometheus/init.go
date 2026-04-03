// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

const (
	profileSelectionModeAuto     = "auto"
	profileSelectionModeExact    = "exact"
	profileSelectionModeCombined = "combined"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' can not be empty")
	}

	mode := normalizeProfileSelectionMode(c.ProfileSelectionMode)
	profiles, err := normalizeConfiguredProfiles(c.Profiles)
	if err != nil {
		return err
	}

	switch mode {
	case profileSelectionModeAuto:
		if len(profiles) != 0 {
			return errors.New("'profiles' must be empty when 'profile_selection_mode' is 'auto'")
		}
	case profileSelectionModeExact, profileSelectionModeCombined:
		if len(profiles) == 0 {
			return fmt.Errorf("'profiles' is required when 'profile_selection_mode' is %q", mode)
		}
	default:
		return fmt.Errorf("unsupported 'profile_selection_mode' %q", mode)
	}

	c.cfgState.profileSelectionMode = mode
	c.cfgState.profiles = profiles
	c.probeState.expectedPrefix = c.ExpectedPrefix
	c.probeState.maxTS = c.MaxTS

	return nil
}

func (c *Collector) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("init HTTP client: %v", err)
	}

	req := c.RequestConfig.Copy()

	sr, err := c.Selector.Parse()
	if err != nil {
		return nil, fmt.Errorf("parsing selector: %v", err)
	}

	if sr != nil {
		return prometheus.NewWithSelector(httpClient, req, sr), nil
	}
	return prometheus.New(httpClient, req), nil
}

func (c *Collector) loadProfilesCatalog() (promprofiles.Catalog, error) {
	if c.loadProfileCatalog == nil {
		c.loadProfileCatalog = promprofiles.DefaultCatalog
	}

	catalog, err := c.loadProfileCatalog()
	if err != nil {
		return promprofiles.Catalog{}, err
	}
	return catalog, nil
}

func (c *Collector) initFallbackTypeMatcher(expr []string) (matcher.Matcher, error) {
	if len(expr) == 0 {
		return nil, nil
	}

	m := matcher.FALSE()

	for _, pattern := range expr {
		v, err := matcher.NewGlobMatcher(pattern)
		if err != nil {
			return nil, fmt.Errorf("error on parsing pattern '%s': %v", pattern, err)
		}
		m = matcher.Or(m, v)
	}

	return m, nil
}

func normalizeProfileSelectionMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return profileSelectionModeAuto
	}
	return mode
}

func normalizeConfiguredProfiles(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, errors.New("'profiles' must not contain empty values")
		}

		key := promprofiles.NormalizeProfileKey(id)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate profile name %q in 'profiles'", id)
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}

	return out, nil
}
