// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import "github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

func validateStoreName(name string) error {
	return dyncfg.JobNameRuleAllowDots(name)
}
