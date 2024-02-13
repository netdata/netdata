// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"errors"
	"strconv"
	"strings"
)

func (nv *NvidiaSMI) collect() (map[string]int64, error) {
	if nv.exec == nil {
		return nil, errors.New("nvidia-smi exec is not initialized")
	}

	mx := make(map[string]int64)

	if err := nv.collectGPUInfo(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (nv *NvidiaSMI) collectGPUInfo(mx map[string]int64) error {
	if nv.UseCSVFormat {
		return nv.collectGPUInfoCSV(mx)
	}
	return nv.collectGPUInfoXML(mx)
}

func addMetric(mx map[string]int64, key, value string, mul int) {
	if !isValidValue(value) {
		return
	}

	value = removeUnits(value)

	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return
	}

	if mul > 0 {
		v *= float64(mul)
	}

	mx[key] = int64(v)
}

func isValidValue(v string) bool {
	return v != "" && v != "N/A" && v != "[N/A]"
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(removeUnits(s), 64)
	return v
}

func removeUnits(s string) string {
	if i := strings.IndexByte(s, ' '); i != -1 {
		s = s[:i]
	}
	return s
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
