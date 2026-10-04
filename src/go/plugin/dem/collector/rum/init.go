// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
)

var siteKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.deps.Hub == nil || c.deps.History == nil {
		return errors.New("missing runtime/history dependencies")
	}
	if !siteKey.MatchString(c.Name) {
		return errors.New("name must be a nonempty URL-safe site key: letters, digits, underscores or hyphens")
	}
	if time.Duration(c.Window) < time.Minute || time.Duration(c.Window) > 30*time.Minute {
		return errors.New("window must be between 1m and 30m")
	}
	if c.PageGroups < 1 || c.PageGroups > 100 || c.Countries < 1 || c.Countries > 100 {
		return errors.New("page_groups and countries must be between 1 and 100")
	}
	if len(c.AllowedOrigins) == 0 {
		return errors.New("allowed_origins is required")
	}
	for _, origin := range c.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
			(u.Path != "" && u.Path != "/") ||
			u.RawQuery != "" ||
			u.Fragment != "" {
			return fmt.Errorf("allowed origin %q must be http(s)://host[:port]", origin)
		}
		host := u.Hostname()
		if strings.Contains(host, "*") &&
			(!strings.HasPrefix(host, "*.") || strings.Contains(host[2:], "*") || len(host) == 2) {
			return errors.New("origin wildcard is only allowed as a leading *.")
		}
	}
	if errs := config.ValidateSiteExtras(c.Site); len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	if math.IsNaN(c.MeasureSampleRate) || c.MeasureSampleRate < 0 || c.MeasureSampleRate > 1 {
		return errors.New("measure_sample_rate must be between 0 and 1")
	}
	if c.Investigate != nil {
		if math.IsNaN(c.Investigate.SampleRate) || c.Investigate.SampleRate < 0 || c.Investigate.SampleRate > 1 {
			return errors.New("investigate sample_rate must be between 0 and 1")
		}
		for _, keep := range c.Investigate.AlwaysKeep {
			if keep != config.KeepErrors && keep != config.KeepPoorVitals {
				return errors.New("unknown investigate always_keep condition")
			}
		}
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
			u.RawQuery != "" ||
			u.Fragment != "" {
			return errors.New("public_url must be an http(s) base URL")
		}
	}
	if c.OTLP.Enabled != "auto" && c.OTLP.Enabled != "yes" && c.OTLP.Enabled != "no" {
		return errors.New("otlp.enabled must be auto, yes or no")
	}
	if err := otlp.ValidateConfig(ctx, c.OTLP); err != nil {
		return err
	}
	c.redactor = secrets.NewRedactor(c.OTLP.AuthToken)
	c.aggregator = agg.New(time.Duration(c.Window), agg.SiteCfg{
		Name:        c.Name,
		DisplayName: c.Label(),
		PageGroups:  c.PageGroups,
		Countries:   c.Countries,
		Investigate: agg.InvestigateCfg{
			Rate:           c.InvestigateRate(),
			KeepErrors:     c.KeepsErrors(),
			KeepPoorVitals: c.KeepsPoorVitals(),
		},
	})
	m := c.store.Write().SnapshotMeter("")
	if c.DisplayName != "" && c.DisplayName != c.Name {
		m = m.WithLabels(metrix.Label{
			Key:   "display_name",
			Value: c.redactor.Apply(c.DisplayName),
		})
	}
	c.metrics = newCollectorMetrics(m)
	return nil
}
