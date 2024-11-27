// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package litespeed

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const precision = 100

func (c *Collector) collect() (map[string]int64, error) {
	if c.checkDir {
		_, err := os.Stat(c.ReportsDir)
		if err != nil {
			return nil, err
		}
		c.checkDir = false
	}
	reports, err := filepath.Glob(filepath.Join(c.ReportsDir, ".rtreport*"))
	if err != nil {
		return nil, err
	}

	c.Debugf("found %d reports: %v", len(reports), reports)

	if len(reports) == 0 {
		return nil, errors.New("no reports found")
	}

	mx := make(map[string]int64)

	for _, report := range reports {
		if err := c.collectReport(mx, report); err != nil {
			return nil, err
		}
	}

	return mx, nil
}

func (c *Collector) collectReport(mx map[string]int64, filename string) error {
	bs, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	sc := bufio.NewScanner(bytes.NewReader(bs))

	var valid bool

	for sc.Scan() {
		line := sc.Text()

		switch {
		default:
			continue
		case strings.HasPrefix(line, "BPS_IN:"):
		case strings.HasPrefix(line, "PLAINCONN:"):
		case strings.HasPrefix(line, "MAXCONN:"):
		case strings.HasPrefix(line, "REQ_RATE []:"):
			line = strings.TrimPrefix(line, "REQ_RATE []:")
		}

		parts := strings.Split(line, ",")

		for _, part := range parts {
			i := strings.IndexByte(part, ':')
			if i == -1 {
				c.Debugf("Skipping metric '%s': missing colon separator", part)
				continue
			}

			metric, sVal := strings.TrimSpace(part[:i]), strings.TrimSpace(part[i+1:])

			val, err := strconv.ParseFloat(sVal, 64)
			if err != nil {
				c.Debugf("Skipping metric '%s': invalid value", part)
				continue
			}

			key := strings.ToLower(metric)

			switch metric {
			default:
				continue
			case "REQ_PER_SEC",
				"PUB_CACHE_HITS_PER_SEC",
				"PRIVATE_CACHE_HITS_PER_SEC",
				"STATIC_HITS_PER_SEC":
				mx[key] += int64(val * precision)
			case "BPS_IN",
				"BPS_OUT",
				"SSL_BPS_IN",
				"SSL_BPS_OUT":
				mx[key] += int64(val) * 8
			case "REQ_PROCESSING",
				"PLAINCONN",
				"AVAILCONN",
				"SSLCONN",
				"AVAILSSL":
				mx[key] += int64(val)
			}
			valid = true

		}
	}

	if !valid {
		return errors.New("unexpected file: not a litespeed report")
	}

	return nil
}
