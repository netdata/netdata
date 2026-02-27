// SPDX-License-Identifier: GPL-3.0-or-later

package metricsaudit

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

// Analyzer captures metrics-audit hooks used by job manager and runtime jobs.
type Analyzer interface {
	RegisterJob(jobName, moduleName, dir string)
	RecordJobStructure(jobName, moduleName string, charts *collectorapi.Charts)
	UpdateJobStructure(jobName, moduleName string, charts *collectorapi.Charts)
	RecordCollection(jobName, moduleName string, mx map[string]int64)
}

// Capturable marks collectors that can emit additional capture artifacts.
type Capturable interface {
	EnableCaptureArtifacts(string)
}
