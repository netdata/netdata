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
	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
)

var siteKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.deps.Registry == nil || c.deps.History == nil {
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
	c.Site = c.Site.Effective()
	if errs := config.ValidateSiteExtras(c.Site); len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	if math.IsNaN(c.MeasureRate()) || c.MeasureRate() < 0 || c.MeasureRate() > 1 {
		return errors.New("measure_sample_rate must be between 0 and 1")
	}
	if c.Investigate != nil {
		if math.IsNaN(c.InvestigateRate()) || c.InvestigateRate() < 0 || c.InvestigateRate() > 1 {
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
	if c.EventLogsOn() {
		if err := otlp.ValidateDestination(ctx, c.EventLogs.Destination); err != nil {
			return fmt.Errorf("event_logs.destination: %w", err)
		}
	}
	if c.TracingOn() {
		if err := otlp.ValidateDestination(ctx, c.Tracing.Destination); err != nil {
			return fmt.Errorf("tracing.destination: %w", err)
		}
	}
	// Retained inactive credentials still need output redaction.
	c.redactor = redact.NewRedactor(c.EventLogs.Destination.AuthToken, c.Tracing.Destination.AuthToken)
	c.aggregator = aggregate.New(time.Duration(c.Window), aggregate.SiteCfg{
		Name:               c.Name,
		DisplayName:        c.Label(),
		PageGroups:         c.PageGroups,
		Countries:          c.Countries,
		FrustrationSignals: c.FrustrationSignalsOn(),
		Investigate: aggregate.InvestigateCfg{
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
	c.metrics = newCollectorMetrics(m, c.EventLogsOn(), c.TracingOn(), c.FrustrationSignalsOn())
	return nil
}
