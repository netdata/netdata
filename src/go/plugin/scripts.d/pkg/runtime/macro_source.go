// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"

// MacroSource exposes the data required by the macro builder.
type MacroSource interface {
	JobSpecs() []spec.JobSpec
	UserMacros() map[string]string
	VnodeInfo(job spec.JobSpec) VnodeInfo
}
