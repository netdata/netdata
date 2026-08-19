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

	return errors.Join(errs...)
}
