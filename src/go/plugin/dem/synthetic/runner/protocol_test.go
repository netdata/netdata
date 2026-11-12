// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"

// frame builds baseline fixture output independently of the live wire decoder.
type frame struct {
	Type   string           `json:"type"`
	Event  *synthetic.Event `json:"event,omitempty"`
	Result *reporterResult  `json:"result,omitempty"`
}
