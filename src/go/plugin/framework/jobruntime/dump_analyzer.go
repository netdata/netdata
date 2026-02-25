// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

// DumpAnalyzer captures dump-mode hooks used by job manager and runtime job.
// Implementations can persist per-job artifacts and summarize metric structures.
type DumpAnalyzer interface {
	RegisterJob(jobName, moduleName, dir string)
	RecordJobStructure(jobName, moduleName string, charts *collectorapi.Charts)
	UpdateJobStructure(jobName string, charts *collectorapi.Charts)
	RecordCollection(jobName string, mx map[string]int64)
}
