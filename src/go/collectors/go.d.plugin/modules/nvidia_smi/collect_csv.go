// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// use of property aliases is not implemented ('"<property>" or "<alias>"' in help-query-gpu)
var knownProperties = map[string]bool{
	"uuid":                    true,
	"name":                    true,
	"fan.speed":               true,
	"pstate":                  true,
	"utilization.gpu":         true,
	"utilization.memory":      true,
	"memory.used":             true,
	"memory.free":             true,
	"memory.reserved":         true,
	"temperature.gpu":         true,
	"clocks.current.graphics": true,
	"clocks.current.video":    true,
	"clocks.current.sm":       true,
	"clocks.current.memory":   true,
	"power.draw":              true,
}

var reHelpProperty = regexp.MustCompile(`"([a-zA-Z_.]+)"`)

func (nv *NvidiaSMI) collectGPUInfoCSV(mx map[string]int64) error {
	if len(nv.gpuQueryProperties) == 0 {
		bs, err := nv.exec.queryHelpQueryGPU()
		if err != nil {
			return err
		}

		sc := bufio.NewScanner(bytes.NewBuffer(bs))

		for sc.Scan() {
			if !strings.HasPrefix(sc.Text(), "\"") {
				continue
			}
			matches := reHelpProperty.FindAllString(sc.Text(), -1)
			if len(matches) == 0 {
				continue
			}
			for _, v := range matches {
				if v = strings.Trim(v, "\""); knownProperties[v] {
					nv.gpuQueryProperties = append(nv.gpuQueryProperties, v)
				}
			}
		}
		nv.Debugf("found query GPU properties: %v", nv.gpuQueryProperties)
	}

	bs, err := nv.exec.queryGPUInfoCSV(nv.gpuQueryProperties)
	if err != nil {
		return err
	}

	nv.Debugf("GPU info:\n%s", bs)

	r := csv.NewReader(bytes.NewBuffer(bs))
	r.Comma = ','
	r.ReuseRecord = true
	r.TrimLeadingSpace = true

	// skip headers
	if _, err := r.Read(); err != nil && err != io.EOF {
		return err
	}

	var gpusInfo []csvGPUInfo
	for {
		record, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		if len(record) != len(nv.gpuQueryProperties) {
			return fmt.Errorf("record values (%d) != queried properties (%d)", len(record), len(nv.gpuQueryProperties))
		}

		var gpu csvGPUInfo
		for i, v := range record {
			switch nv.gpuQueryProperties[i] {
			case "uuid":
				gpu.uuid = v
			case "name":
				gpu.name = v
			case "fan.speed":
				gpu.fanSpeed = v
			case "pstate":
				gpu.pstate = v
			case "utilization.gpu":
				gpu.utilizationGPU = v
			case "utilization.memory":
				gpu.utilizationMemory = v
			case "memory.used":
				gpu.memoryUsed = v
			case "memory.free":
				gpu.memoryFree = v
			case "memory.reserved":
				gpu.memoryReserved = v
			case "temperature.gpu":
				gpu.temperatureGPU = v
			case "clocks.current.graphics":
				gpu.clocksCurrentGraphics = v
			case "clocks.current.video":
				gpu.clocksCurrentVideo = v
			case "clocks.current.sm":
				gpu.clocksCurrentSM = v
			case "clocks.current.memory":
				gpu.clocksCurrentMemory = v
			case "power.draw":
				gpu.powerDraw = v
			}
		}
		gpusInfo = append(gpusInfo, gpu)
	}

	seen := make(map[string]bool)

	for _, gpu := range gpusInfo {
		if !isValidValue(gpu.uuid) || !isValidValue(gpu.name) {
			continue
		}

		px := "gpu_" + gpu.uuid + "_"

		seen[px] = true

		if !nv.gpus[px] {
			nv.gpus[px] = true
			nv.addGPUCSVCharts(gpu)
		}

		addMetric(mx, px+"fan_speed_perc", gpu.fanSpeed, 0)
		addMetric(mx, px+"gpu_utilization", gpu.utilizationGPU, 0)
		addMetric(mx, px+"mem_utilization", gpu.utilizationMemory, 0)
		addMetric(mx, px+"frame_buffer_memory_usage_free", gpu.memoryFree, 1024*1024)         // MiB => bytes
		addMetric(mx, px+"frame_buffer_memory_usage_used", gpu.memoryUsed, 1024*1024)         // MiB => bytes
		addMetric(mx, px+"frame_buffer_memory_usage_reserved", gpu.memoryReserved, 1024*1024) // MiB => bytes
		addMetric(mx, px+"temperature", gpu.temperatureGPU, 0)
		addMetric(mx, px+"graphics_clock", gpu.clocksCurrentGraphics, 0)
		addMetric(mx, px+"video_clock", gpu.clocksCurrentVideo, 0)
		addMetric(mx, px+"sm_clock", gpu.clocksCurrentSM, 0)
		addMetric(mx, px+"mem_clock", gpu.clocksCurrentMemory, 0)
		addMetric(mx, px+"power_draw", gpu.powerDraw, 0)
		for i := 0; i < 16; i++ {
			if s := "P" + strconv.Itoa(i); gpu.pstate == s {
				mx[px+"performance_state_"+s] = 1
			} else {
				mx[px+"performance_state_"+s] = 0
			}
		}
	}

	for px := range nv.gpus {
		if !seen[px] {
			delete(nv.gpus, px)
			nv.removeCharts(px)
		}
	}

	return nil
}

type (
	csvGPUInfo struct {
		uuid                  string
		name                  string
		fanSpeed              string
		pstate                string
		utilizationGPU        string
		utilizationMemory     string
		memoryUsed            string
		memoryFree            string
		memoryReserved        string
		temperatureGPU        string
		clocksCurrentGraphics string
		clocksCurrentVideo    string
		clocksCurrentSM       string
		clocksCurrentMemory   string
		powerDraw             string
	}
)
