// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package zfspool

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type zpoolEntry struct {
	name       string
	sizeBytes  string
	allocBytes string
	freeBytes  string
	fragPerc   string
	capPerc    string
	dedupRatio string
	health     string
}

func (c *Collector) collectZpoolList(mx map[string]int64) error {
	bs, err := c.exec.list()
	if err != nil {
		return err
	}

	zpools, err := parseZpoolListOutput(bs)
	if err != nil {
		return fmt.Errorf("bad zpool list output: %v", err)
	}

	seen := make(map[string]bool)

	for _, zpool := range zpools {
		seen[zpool.name] = true

		if !c.seenZpools[zpool.name] {
			c.addZpoolCharts(zpool.name)
			c.seenZpools[zpool.name] = true
		}

		px := "zpool_" + zpool.name + "_"

		if v, ok := parseInt(zpool.sizeBytes); ok {
			mx[px+"size"] = v
		}
		if v, ok := parseInt(zpool.freeBytes); ok {
			mx[px+"free"] = v
		}
		if v, ok := parseInt(zpool.allocBytes); ok {
			mx[px+"alloc"] = v
		}
		if v, ok := parseFloat(zpool.capPerc); ok {
			mx[px+"cap"] = int64(v)
		}
		if v, ok := parseFloat(zpool.fragPerc); ok {
			mx[px+"frag"] = int64(v)
		}
		for _, s := range zpoolHealthStates {
			mx[px+"health_state_"+s] = 0
		}
		mx[px+"health_state_"+zpool.health] = 1
	}

	for name := range c.seenZpools {
		if !seen[name] {
			c.removeZpoolCharts(name)
			delete(c.seenZpools, name)
		}
	}

	return nil
}

func parseZpoolListOutput(bs []byte) ([]zpoolEntry, error) {
	/*
	   # zpool list -p
	   NAME          SIZE       ALLOC         FREE  EXPANDSZ   FRAG    CAP  DEDUP  HEALTH  ALTROOT
	   rpool  21367462298  9051643576  12240656794         -     33     42   1.00  ONLINE  -
	   zion             -           -            -         -      -      -      -  FAULTED -
	*/

	var headers []string
	var zpools []zpoolEntry
	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
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
		if len(values) != len(headers) {
			return nil, fmt.Errorf("unequal columns: headers(%d) != values(%d)", len(headers), len(values))
		}

		var zpool zpoolEntry

		for i, v := range values {
			v = strings.TrimSpace(v)
			switch strings.ToLower(headers[i]) {
			case "name":
				zpool.name = v
			case "size":
				zpool.sizeBytes = v
			case "alloc":
				zpool.allocBytes = v
			case "free":
				zpool.freeBytes = v
			case "frag":
				zpool.fragPerc = v
			case "cap":
				zpool.capPerc = v
			case "dedup":
				zpool.dedupRatio = v
			case "health":
				zpool.health = strings.ToLower(v)
			}
		}

		if zpool.name != "" && zpool.health != "" {
			zpools = append(zpools, zpool)
		}
	}

	if len(zpools) == 0 {
		return nil, errors.New("no pools found")
	}

	return zpools, nil
}

func parseInt(s string) (int64, bool) {
	if s == "-" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

func parseFloat(s string) (float64, bool) {
	if s == "-" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}
