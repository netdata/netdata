// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func managedFocusParamOptions(deps Deps) []funcapi.ParamOption {
	if deps == nil {
		return nil
	}

	options := make([]funcapi.ParamOption, 0)
	for _, target := range deps.ManagedDeviceFocusTargets() {
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
