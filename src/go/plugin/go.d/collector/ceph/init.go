// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func (c *Collector) validateConfig() error {
	var errs []error

	base, err := parseDashboardBaseURL(c.URL)
	if err != nil {
		errs = append(errs, err)
	} else if _, err := parseAllowedRedirectOrigins(base, c.AllowedRedirectOrigins); err != nil {
		errs = append(errs, err)
	}
	if c.BearerTokenFile == "" && (c.Username == "" || c.Password == "") {
		errs = append(errs, fmt.Errorf("username and password, or bearer_token_file, are required"))
	}
	if c.Method != "" {
		errs = append(errs, errors.New("method is not supported by the Ceph collector"))
	}
	if c.Body != "" {
		errs = append(errs, errors.New("body is not supported by the Ceph collector"))
	}
	for name := range c.Headers {
		switch strings.ToLower(name) {
		case "authorization", "cookie", "host":
			errs = append(errs, fmt.Errorf("header %q is managed by the Ceph collector", name))
		}
	}

	if c.MaxOSDs <= 0 {
		errs = append(errs, errors.New("max_osds must be positive"))
	}
	if c.MaxPools <= 0 {
		errs = append(errs, errors.New("max_pools must be positive"))
	}
	if c.OSDSelector == "" {
		errs = append(errs, errors.New("osd_selector is required"))
	}
	if c.PoolSelector == "" {
		errs = append(errs, errors.New("pool_selector is required"))
	}
	if c.Timeout.Duration() < 500*time.Millisecond {
		errs = append(errs, errors.New("timeout must be at least 0.5 seconds"))
	}

	if c.FunctionOnly && c.Metrics.anyEnabled() {
		errs = append(errs, errors.New("function_only cannot be combined with enabled periodic metrics"))
	}
	if !c.FunctionOnly && !c.Metrics.anyEnabled() {
		errs = append(errs, errors.New("enable at least one periodic metric or set function_only to true"))
	}

	for name, cfg := range map[string]FunctionConfig{
		"health":        c.Functions.Health,
		"osds":          c.Functions.OSDs,
		"pools":         c.Functions.Pools,
		"daemons":       c.Functions.Daemons,
		"rgw_multisite": c.Functions.RGWMultisite,
		"rgw_quotas":    c.Functions.RGWQuotas.FunctionConfig,
	} {
		if cfg.Timeout.Duration() < 0 {
			errs = append(errs, fmt.Errorf("functions.%s.timeout must not be negative", name))
		}
		if cfg.Limit <= 0 {
			errs = append(errs, fmt.Errorf("functions.%s.limit must be positive", name))
		}
		if cfg.Limit > maxFunctionRows {
			errs = append(errs, fmt.Errorf("functions.%s.limit must not exceed %d", name, maxFunctionRows))
		}
	}
	quotaTargetCount := len(quotaTargets(c.Functions.RGWQuotas))
	if !c.Functions.RGWQuotas.Disabled && quotaTargetCount == 0 {
		errs = append(errs, errors.New("functions.rgw_quotas requires at least one explicit user, bucket, or account"))
	}
	if quotaTargetCount > c.Functions.RGWQuotas.Limit {
		errs = append(errs, errors.New("functions.rgw_quotas target count exceeds its limit"))
	}

	return errors.Join(errs...)
}
