// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func (v *Varnish) collect() (map[string]int64, error) {
	bs, err := v.exec.varnishStatistics()
	if err != nil {
		v.charts = nil
		return nil, err
	}

	mx, err := v.parseStatistics(bs)

	if err != nil {
		return nil, fmt.Errorf("error in parsing the statistics")
	}

	return mx, nil
}

func (v *Varnish) parseStatistics(resp []byte) (map[string]int64, error) {
	// Global regex for the metric lines
	pattern := `([A-Z]+\.)?([\d\w_.]+)\s+(\d+)`

	mx := make(map[string]int64)
	seenB := make(map[string]bool)
	seenS := make(map[string]bool)

	re, err := regexp.Compile(pattern)
	if err != nil {
		fmt.Println("Failed to compile regex:", err)
		return nil, err
	}

	lines := strings.Split(string(resp), "\n")

	for _, line := range lines {
		match := re.FindSubmatch([]byte(line))

		if len(match) == 4 {
			prefix := string(match[1])
			identifier := string(match[2])
			number := string(match[3])

			if isBackendField(prefix) {
				backendName := strings.Split(identifier, ".")[1]
				metricName := strings.Split(identifier, ".")[2]

				seenB[backendName] = true
				if !v.seenBackends[backendName] {
					v.addBackendCharts(backendName)
					v.seenBackends[backendName] = true
				}
				mx[fmt.Sprintf("%s_%s", backendName, metricName)], err = parseInt(number)

			} else if isStorageField(prefix) {
				storageName := strings.Split(identifier, ".")[0]
				metricName := strings.Split(identifier, ".")[1]

				seenS[storageName] = true
				if !v.seenStorages[storageName] {
					v.addStorageCharts(storageName)
					v.seenStorages[storageName] = true
				}

				mx[fmt.Sprintf("%s_%s", storageName, metricName)], err = parseInt(number)
			} else {
				mx[identifier], err = parseInt(number)
			}

			if err != nil {
				return nil, fmt.Errorf("couldn't parse int from number")
			}

			for backend := range v.seenBackends {
				if !seenB[backend] {
					delete(v.seenBackends, backend)
					v.removeBackendCharts(backend)
				}
			}

			for storage := range v.seenStorages {
				if !seenS[storage] {
					delete(v.seenStorages, storage)
					v.removeStorageCharts(storage)
				}
			}
		}
	}
	return mx, nil
}

func isBackendField(field string) bool {
	for _, px := range []string{
		"VBE",
	} {
		if strings.Split(field, ".")[0] == px {
			return true
		}
	}
	return false
}

func isStorageField(field string) bool {
	for _, px := range []string{
		"SMF",
		"SMA",
		"MSE",
	} {
		if strings.Split(field, ".")[0] == px {
			return true
		}
	}
	return false
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
