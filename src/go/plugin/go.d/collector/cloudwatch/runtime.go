// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

type collectorRuntime struct {
	Targets      []*targetRuntime
	TargetsByRef map[string]*targetRuntime
	Scopes       []collectionScope
	Profiles     []cwprofiles.ResolvedProfile
	Diagnostics  []string
}

type targetRuntime struct {
	Name     string
	Identity awsauth.Identity
	RoleARN  string
	Regions  []string
}

type collectionScope struct {
	RuleName string
	Target   *targetRuntime
	Profile  cwprofiles.ResolvedProfile
	Region   string
}
