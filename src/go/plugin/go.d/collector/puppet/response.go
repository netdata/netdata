// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

type statusServiceResponse struct {
	StatusService *struct {
		Status struct {
			Experimental struct {
				JVMMetrics *struct {
					CPUUsage   float64 `json:"cpu-usage"  stm:"cpu_usage,1000,1"`
					GCCPUUsage float64 `json:"gc-cpu-usage"  stm:"gc_cpu_usage,1000,1"`
					HeapMemory struct {
						Committed int64 `json:"committed"  stm:"committed"`
						Init      int64 `json:"init"  stm:"init"`
						Max       int64 `json:"max"  stm:"max"`
						Used      int64 `json:"used"  stm:"used"`
					} `json:"heap-memory"  stm:"jvm_heap"`
					FileDescriptors struct {
						Used int `json:"used"  stm:"used"`
						Max  int `json:"max"  stm:"max"`
					} `json:"file-descriptors"  stm:"fd"`
					NonHeapMemory struct {
						Committed int64 `json:"committed"  stm:"committed"`
						Init      int64 `json:"init"  stm:"init"`
						Max       int64 `json:"max"  stm:"max"`
						Used      int64 `json:"used"  stm:"used"`
					} `json:"non-heap-memory"  stm:"jvm_nonheap"`
				} `json:"jvm-metrics"  stm:""`
			} `json:"experimental" stm:""`
		} `json:"status"  stm:""`
	} `json:"status-service" stm:""`
}
