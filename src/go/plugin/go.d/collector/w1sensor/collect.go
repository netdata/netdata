// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package w1sensor

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const precision = 10

func (c *Collector) collect() (map[string]int64, error) {
	des, err := os.ReadDir(c.SensorsPath)
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)
	seen := make(map[string]bool)

	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		if !isW1sensorDir(de.Name()) {
			c.Debugf("'%s' is not a w1sensor directory, skipping it", filepath.Join(c.SensorsPath, de.Name()))
			continue
		}

		filename := filepath.Join(c.SensorsPath, de.Name(), "w1_slave")

		temp, err := readW1sensorTemperature(filename)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				c.Debugf("'%s' doesn't have 'w1_slave', skipping it", filepath.Join(c.SensorsPath, de.Name()))
				continue
			}
			return nil, fmt.Errorf("failed to read temperature from '%s': %w", filename, err)
		}

		seen[de.Name()] = true
		if !c.seenSensors[de.Name()] {
			c.addSensorChart(de.Name())

		}

		mx[fmt.Sprintf("w1sensor_%s_temperature", de.Name())] = temp
	}

	for id := range c.seenSensors {
		if !seen[id] {
			delete(c.seenSensors, id)
			c.removeSensorChart(id)
		}
	}

	if len(mx) == 0 {
		return nil, errors.New("no w1 sensors found")
	}

	return mx, nil
}

func readW1sensorTemperature(filename string) (int64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	sc := bufio.NewScanner(file)
	sc.Scan()
	// The second line displays the retained values along with a temperature in milli degrees Centigrade after t=.
	sc.Scan()

	_, tempStr, ok := strings.Cut(strings.TrimSpace(sc.Text()), "t=")
	if !ok {
		return 0, errors.New("no temperature found")
	}

	v, err := strconv.ParseInt(tempStr, 10, 64)
	if err != nil {
		return 0, err
	}

	return int64(float64(v) / 1000 * precision), nil
}

func isW1sensorDir(dirName string) bool {
	// Supported family members
	// Based on linux/drivers/w1/w1_family.h and w1/slaves/w1_therm.c
	for _, px := range []string{
		"10-", // W1_THERM_DS18S20
		"22-", // W1_THERM_DS1822
		"28-", // W1_THERM_DS18B20
		"3b-", // W1_THERM_DS1825
		"42-", // W1_THERM_DS28EA00
	} {
		if strings.HasPrefix(dirName, px) {
			return true
		}
	}
	return false
}
