// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	stats, err := c.exec.stats()
	if err != nil {
		return nil, err
	}

	if len(stats) == 0 {
		return nil, errors.New("empty stats response")
	}

	mx := make(map[string]int64)

	sc := bufio.NewScanner(bytes.NewReader(stats))

	for sc.Scan() {
		c.collectStatsLine(mx, sc.Text())
	}

	if len(mx) == 0 {
		return nil, errors.New("unexpected stats response: no metrics found")
	}

	addMissingMetrics(mx, "num.rcode.", answerRcodes)
	addMissingMetrics(mx, "num.opcode.", queryOpcodes)
	addMissingMetrics(mx, "num.class.", queryClasses)
	addMissingMetrics(mx, "num.type.", queryTypes)

	return mx, nil
}

func (c *Collector) collectStatsLine(mx map[string]int64, line string) {
	if line = strings.TrimSpace(line); line == "" {
		return
	}

	key, value, ok := strings.Cut(line, "=")
	if !ok {
		c.Debugf("invalid line in stats: '%s'", line)
		return
	}

	var v int64
	var f float64
	var err error

	switch key {
	case "time.boot":
		f, err = strconv.ParseFloat(value, 64)
		v = int64(f)
	default:
		v, err = strconv.ParseInt(value, 10, 64)
	}

	if err != nil {
		c.Debugf("invalid value in stats line '%s': '%s'", line, value)
		return
	}

	mx[key] = v
}

func addMissingMetrics(mx map[string]int64, prefix string, values []string) {
	for _, v := range values {
		k := prefix + v
		if _, ok := mx[k]; !ok {
			mx[k] = 0
		}
	}
}
