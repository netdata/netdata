// SPDX-License-Identifier: GPL-3.0-or-later

package w1sensor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Known and supported family members
// Based on linux/drivers/w1/w1_family.h and w1/slaves/w1_therm.c
var THERM_FAMILY = map[string]string{
	"10": "W1_THERM_DS18S20",
	"22": "W1_THERM_DS1822",
	"28": "W1_THERM_DS18B20",
	"3b": "W1_THERM_DS1825",
	"42": "W1_THERM_DS28EA00",
}

func readFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var content string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		content += scanner.Text() + "\n"
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return content, nil
}

func (w *W1sensor) collect() (map[string]int64, error) {
	mx := make(map[string]int64)
	seen := make(map[string]bool)

	err := filepath.Walk(w.SensorsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			filePath := filepath.Join(path, "w1_slave")
			parentDir := filepath.Base(filepath.Dir(filePath))

			valid := false
			for key := range THERM_FAMILY {
				if strings.HasPrefix(parentDir, key) {
					valid = true
					break
				}
			}

			if valid {

				if _, err := os.Stat(filePath); err == nil {
					content, err := readFile(filePath)

					if err != nil {
						return fmt.Errorf("failed to read file: %s", err)
					}
					pattern := ` t=(-?\d+)`

					re, err := regexp.Compile(pattern)
					if err != nil {
						return fmt.Errorf("failed to compile regex: %s", err)
					}

					matches := re.FindAllStringSubmatch(content, -1)

					if len(matches) == 1 {
						seen[parentDir] = true

						if _, ok := w.seenSensors[parentDir]; !ok {
							w.seenSensors[parentDir] = parentDir
							w.addSensorChart(parentDir)
						}

						mx[fmt.Sprintf("w1sensor_temp_%s", parentDir)], err = parseInt(matches[0][1])
						if err != nil {
							return fmt.Errorf("failed to parse temp %s from sensor %s", matches[1], parentDir)
						}
					} else {
						return fmt.Errorf("no temperature found for the sensor")
					}
				}
			}
		}

		return nil
	})

	for key, sensor := range w.seenSensors {
		if !seen[key] {
			delete(w.seenSensors, key)
			w.removeSensorChart(sensor)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("error walking the path: %v", err)
	}

	return mx, nil
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
