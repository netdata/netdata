// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

func (c *Collector) initIntelGPUTopExec() (intelGpuTop, error) {
	return newIntelGpuTopExec(c.Logger, c.UpdateEvery, c.Device)
}
