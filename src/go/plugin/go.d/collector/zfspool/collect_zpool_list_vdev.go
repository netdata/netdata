// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package zfspool

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

type vdevEntry struct {
	name   string
	vdev   string // The full path of the vdev within the zpool hierarchy.
	health string

	// Represents the nesting level of the vdev within the zpool hierarchy, based on indentation.
	// A level of -1 indicates the root vdev (the pool itself).
	level int
}

func (c *Collector) collectZpoolListVdev(mx map[string]int64) error {
	seen := make(map[string]bool)

	for pool := range c.seenZpools {
		bs, err := c.exec.listWithVdev(pool)
		if err != nil {
			return err
		}

		vdevs, err := parseZpoolListVdevOutput(bs)
		if err != nil {
			return fmt.Errorf("bad zpool list vdev output (pool '%s'): %v", pool, err)
		}

		for _, vdev := range vdevs {
			if vdev.health == "" || vdev.health == "-" {
				continue
			}

			seen[vdev.vdev] = true
			if !c.seenVdevs[vdev.vdev] {
				c.seenVdevs[vdev.vdev] = true
				c.addVdevCharts(pool, vdev.vdev)
			}

			px := fmt.Sprintf("vdev_%s_", vdev.vdev)

			for _, s := range zpoolHealthStates {
				mx[px+"health_state_"+s] = 0
			}
			mx[px+"health_state_"+vdev.health] = 1
		}
	}

	for name := range c.seenVdevs {
		if !seen[name] {
			c.removeVdevCharts(name)
			delete(c.seenVdevs, name)
		}
	}

	return nil
}

func parseZpoolListVdevOutput(bs []byte) ([]vdevEntry, error) {
	var headers []string
	var vdevs []vdevEntry
	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}

		if len(headers) == 0 {
			if !strings.HasPrefix(line, "NAME") {
				return nil, fmt.Errorf("missing headers (line '%s')", line)
			}
			headers = strings.Fields(line)
			continue
		}

		values := strings.Fields(line)
		if len(values) == 0 || len(values) > len(headers) {
			return nil, fmt.Errorf("unexpected columns: headers(%d)  values(%d) (line '%s')", len(headers), len(values), line)
		}

		vdev := vdevEntry{
			level: len(line) - len(strings.TrimLeft(line, " ")),
		}

		for i, v := range values {
			switch strings.ToLower(headers[i]) {
			case "name":
				vdev.name = v
			case "health":
				vdev.health = strings.ToLower(v)
			}
		}

		if vdev.name != "" {
			if len(vdevs) == 0 {
				vdev.level = -1 // Pool
			}
			vdevs = append(vdevs, vdev)
		}
	}

	// set parent/child relationships
	for i := range vdevs {
		v := &vdevs[i]

		switch i {
		case 0:
			v.vdev = v.name
		default:
			// find parent with a lower level
			for j := i - 1; j >= 0; j-- {
				if vdevs[j].level < v.level {
					v.vdev = fmt.Sprintf("%s/%s", vdevs[j].vdev, v.name)
					break
				}
			}
			if v.vdev == "" {
				return nil, fmt.Errorf("no parent for vdev '%s'", v.name)
			}
		}
	}

	// first is Pool
	if len(vdevs) < 2 {
		return nil, fmt.Errorf("no vdevs found")
	}

	return vdevs[1:], nil
}
