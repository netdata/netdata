// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	_ "embed"
	"errors"
	"net/netip"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/jobruntime"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store    metrix.CollectorStore
	services *pluginServices
	job      *jobruntime.Job
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(ctx context.Context) error {
	validated, err := validateConfig(c.Config)
	if err != nil {
		return dyncfgConfigError(err)
	}
	c.warnCatchAllTrustedRelays(validated.trustedRelays)

	if c.job != nil {
		return nil
	}

	manager, commitCatalog := c.services.catalogCandidate()
	job := jobruntime.New(validated.runtime, c.services.dependencies(c, manager))
	if err := job.Start(ctx, func() {
		c.Versions = append([]string(nil), validated.versions...)
		commitCatalog()
		c.job = job
	}); err != nil {
		switch jobruntime.KindOf(err) {
		case jobruntime.ErrorConfig:
			return dyncfgConfigError(err)
		default:
			return dyncfgStartupError(err)
		}
	}
	return nil
}

func (c *Collector) Check(context.Context) error {
	return nil
}

func (c *Collector) Collect(context.Context) error {
	if c.job == nil {
		return errors.New("receiver not ready")
	}
	return c.job.Collect(c.store)
}

func (c *Collector) Cleanup(context.Context) {
	if c.job == nil {
		return
	}
	c.job.Cleanup()
	c.job = nil
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string {
	if c.job != nil {
		if template := c.job.ChartTemplateYAML(); template != "" {
			return template
		}
	}
	return chartTemplateYAML
}

func (c *Collector) warnCatchAllTrustedRelays(prefixes []netip.Prefix) {
	for _, prefix := range prefixes {
		if trustedRelayPrefixIsCatchAll(prefix) && c.Logger != nil {
			c.Warningf(
				"SNMP trap source.trusted_relays contains catch-all prefix %s; every UDP peer in this address family may override source identity via snmpTrapAddress.0",
				prefix,
			)
		}
	}
}

func trustedRelayPrefixIsCatchAll(prefix netip.Prefix) bool {
	return prefix.Bits() == 0
}

type dyncfgCodedError struct {
	err       error
	code      int
	retryable bool
}

func dyncfgConfigError(err error) *dyncfgCodedError {
	return &dyncfgCodedError{err: err, code: 422}
}

func dyncfgStartupError(err error) *dyncfgCodedError {
	return &dyncfgCodedError{err: err, code: 503, retryable: true}
}

func (e *dyncfgCodedError) Error() string         { return e.err.Error() }
func (e *dyncfgCodedError) Unwrap() error         { return e.err }
func (e *dyncfgCodedError) DyncfgCode() int       { return e.code }
func (e *dyncfgCodedError) DyncfgRetryable() bool { return e.retryable }
