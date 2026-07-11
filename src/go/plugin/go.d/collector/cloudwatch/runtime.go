// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

type collectionPlan struct {
	Targets  []*collectionTarget
	Scopes   []collectionScope
	Profiles []cwprofiles.ResolvedProfile
}

type collectionTarget struct {
	Name     string
	Identity awsauth.Identity
	Regions  []string
}

type collectionScope struct {
	Target  *collectionTarget
	Profile cwprofiles.ResolvedProfile
	Region  string
}

func (c *Collector) ensurePlan() error {
	if c.plan != nil {
		return nil
	}
	catalog, err := c.loadCatalog()
	if err != nil {
		return fmt.Errorf("load CloudWatch profiles: %w", err)
	}
	plan, diagnostics, err := compileConfig(c.Config, catalog)
	if err != nil {
		return fmt.Errorf("compile CloudWatch collection plan: %w", err)
	}
	template, err := buildChartTemplate(plan.Profiles)
	if err != nil {
		return fmt.Errorf("build CloudWatch chart template: %w", err)
	}
	for _, diagnostic := range diagnostics {
		c.Warningf("CloudWatch collection plan: %s", diagnostic)
	}
	c.plan = plan
	c.invalidateQueryPlan()
	c.chartTemplateYAML = template
	c.Infof("CloudWatch: compiled %d collection scope(s) across %d target(s) and %d profile(s)",
		len(plan.Scopes), len(plan.Targets), len(plan.Profiles))
	c.Debugf("CloudWatch tuning: update_every=%ds, discovery.refresh_every=%ds, query_offset=%ds, recently_active_only=%v",
		c.UpdateEvery, c.Discovery.RefreshEvery, c.QueryOffset, c.recentlyActiveOnly())
	return nil
}
