// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func managedFocusParamOptions(deps Deps) []funcapi.ParamOption {
	if deps == nil {
		return nil
	}

	return managedFocusTargetOptions(deps.ManagedDeviceFocusTargets())
}

func managedFocusTargetOptions(targets []topologyoptions.ManagedFocusTarget) []funcapi.ParamOption {
	options := make([]funcapi.ParamOption, 0, len(targets))
	for _, target := range targets {
		if strings.TrimSpace(target.Value) == "" {
			continue
		}
		options = append(options, funcapi.ParamOption{
			ID:   target.Value,
			Name: target.Name,
		})
	}
	return options
}
