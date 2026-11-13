// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"

// SupportedContract is the semantic execution contract accepted by this Agent.
// Runtime releases own component versions and private dependency layouts.
const SupportedContract = synthetic.ExecutionContract
