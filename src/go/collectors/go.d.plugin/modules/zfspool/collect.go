// SPDX-License-Identifier: GPL-3.0-or-later

package zfspool

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

var zpoolHealthStates = []string{
	"online",
	"degraded",
	"faulted",
	"offline",
	"removed",
	"unavail",
	"suspended",
}

type zpoolStats struct {
	name       string
	sizeBytes  string
	allocBytes string
	freeBytes  string
	fragPerc   string
	capPerc    string
	dedupRatio string
	health     string
}

func (z *ZFSPool) collect() (map[string]int64, error) {
	bs, err := z.exec.list()
	if err != nil {
		return nil, err
	}

	zpools, err := parseZpoolListOutput(bs)
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	z.collectZpoolListStats(mx, zpools)

	return mx, nil
}

func (z *ZFSPool) collectZpoolListStats(mx map[string]int64, zpools []zpoolStats) {
	seen := make(map[string]bool)

	for _, zpool := range zpools {
		seen[zpool.name] = true

		if !z.zpools[zpool.name] {
			z.addZpoolCharts(zpool.name)
			z.zpools[zpool.name] = true
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

	for name := range z.zpools {
		if !seen[name] {
			z.removeZpoolCharts(name)
			delete(z.zpools, name)
		}
	}
}

func parseZpoolListOutput(bs []byte) ([]zpoolStats, error) {
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(bs))
	for sc.Scan() {
		if text := strings.TrimSpace(sc.Text()); text != "" {
			lines = append(lines, text)
		}

	}
	if len(lines) < 2 {
		return nil, fmt.Errorf("unexpected data: wanted >= 2 lines, got %d", len(lines))
	}

	headers := strings.Fields(lines[0])
	if len(headers) == 0 {
		return nil, fmt.Errorf("unexpected data: missing headers")
	}

	var zpools []zpoolStats

	/*
	   # zpool list -p
	   NAME          SIZE       ALLOC         FREE  EXPANDSZ   FRAG    CAP  DEDUP  HEALTH  ALTROOT
	   rpool  21367462298  9051643576  12240656794         -     33     42   1.00  ONLINE  -
	   zion             -           -            -         -      -      -      -  FAULTED -
	*/

	for _, line := range lines[1:] {
		values := strings.Fields(line)
		if len(values) != len(headers) {
			return nil, fmt.Errorf("unequal columns: headers(%d) != values(%d)", len(headers), len(values))
		}

		var zpool zpoolStats

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

			if last := i+1 == len(headers); last && zpool.name != "" && zpool.health != "" {
				zpools = append(zpools, zpool)
			}
		}
	}

	if len(zpools) == 0 {
		return nil, fmt.Errorf("unexpected data: missing pools")
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
