// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const licenseNeverExpires = int64(-1)

type licenseInfoResult struct {
	Licenses *licenseEntries `xml:"licenses"`
}

type licenseEntries struct {
	Entries []licenseEntry `xml:"entry"`
}

type licenseEntry struct {
	Feature     string `xml:"feature"`
	Description string `xml:"description"`
	Expires     string `xml:"expires"`
	Expired     string `xml:"expired"`
}

func (c *Collector) collectLicenseMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, licenseInfoCommand)
	if err != nil {
		return false, fmt.Errorf("licenses metricset: %s API call: %w", panosCommandName(licenseInfoCommand), err)
	}

	licenses, found, err := parseLicenses(body)
	if err != nil {
		return false, fmt.Errorf("licenses metricset: %s response: %w", panosCommandName(licenseInfoCommand), err)
	}
	if !found {
		return false, fmt.Errorf("licenses metricset: %s response: %w", panosCommandName(licenseInfoCommand), missingPANOSResultError{expected: "<licenses>"})
	}

	var expired int64
	var errs []error
	for _, entry := range licenses {
		labels := licenseLabelValues(entry)
		isExpired, err := c.licenseExpiredStatus(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf("license %s expired status: %w", firstNonEmpty(entry.Feature, "unknown"), err))
		} else {
			observeStateSetVec(c.metrics.lic.status, boolState(!isExpired, "valid", "expired"), labels...)
		}
		if err == nil && isExpired {
			expired++
			continue
		}

		days, ok, err := c.licenseDaysUntilExpiration(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf("license %s expiration: %w", firstNonEmpty(entry.Feature, "unknown"), err))
			continue
		}
		if ok {
			c.metrics.lic.timeUntilExpiration.WithLabelValues(labels...).Observe(float64(days))
		}
	}

	c.metrics.lic.countTotal.Observe(float64(len(licenses)))
	c.metrics.lic.countExpired.Observe(float64(expired))
	return true, errors.Join(errs...)
}

func parseLicenses(body []byte) ([]licenseEntry, bool, error) {
	var result licenseInfoResult
	if err := decodePANOSResult(body, "PAN-OS licenses response", &result); err != nil {
		return nil, false, err
	}
	if result.Licenses == nil {
		return nil, false, nil
	}
	return result.Licenses.Entries, true, nil
}

func (c *Collector) licenseExpiredStatus(entry licenseEntry) (bool, error) {
	raw := strings.TrimSpace(entry.Expired)
	switch strings.ToLower(raw) {
	case "yes", "true", "expired":
		return true, nil
	case "no", "false", "valid":
		return false, nil
	case "":
		expires := strings.TrimSpace(entry.Expires)
		if strings.EqualFold(expires, "never") {
			return false, nil
		}
		if expires == "" {
			return false, errors.New("missing status")
		}
		exp, err := parseLicenseExpirationDate(expires)
		if err != nil {
			return false, fmt.Errorf("missing status and %w", err)
		}
		now := c.now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		expireDay := time.Date(exp.Year(), exp.Month(), exp.Day(), 0, 0, 0, 0, time.UTC)
		return expireDay.Before(today), nil
	default:
		return false, fmt.Errorf("invalid status %q", raw)
	}
}

func (c *Collector) licenseDaysUntilExpiration(entry licenseEntry) (int64, bool, error) {
	expires := strings.TrimSpace(entry.Expires)
	if strings.EqualFold(expires, "never") {
		return licenseNeverExpires, true, nil
	}
	if expires == "" {
		return 0, false, errors.New("missing expiration date")
	}
	exp, err := parseLicenseExpirationDate(expires)
	if err != nil {
		return 0, false, err
	}

	now := c.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	expireDay := time.Date(exp.Year(), exp.Month(), exp.Day(), 0, 0, 0, 0, time.UTC)
	days := int64(expireDay.Sub(today).Hours() / 24)
	if days < 0 {
		return 0, false, nil
	}
	return days, true, nil
}

func parseLicenseExpirationDate(expires string) (time.Time, error) {
	exp, err := time.ParseInLocation("January 02, 2006", expires, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid expiration date %q", expires)
	}
	return exp, nil
}
