// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
)

const precision = 1000 // float values multiplier and dimensions divisor

func (c *Collector) collect() (map[string]int64, error) {
	info, err := c.rdb.Info(context.Background(), "all").Result()
	if err != nil {
		return nil, err
	}

	if c.server == "" {
		s, v, err := extractServerVersion(info)
		if err != nil {
			return nil, fmt.Errorf("can not extract server app and version: %v", err)
		}
		c.server, c.version = s, v
		c.Debugf(`server="%s",version="%s"`, s, v)
	}

	if c.server != "redis" {
		return nil, fmt.Errorf("unsupported server app, want=redis, got=%s", c.server)
	}

	mx := make(map[string]int64)
	c.collectInfo(mx, info)
	c.collectPingLatency(mx)

	return mx, nil
}

// redis_version:6.0.9
var reVersion = regexp.MustCompile(`([a-z]+)_version:(\d+\.\d+\.\d+)`)

func extractServerVersion(info string) (string, *semver.Version, error) {
	var versionLine string
	for sc := bufio.NewScanner(strings.NewReader(info)); sc.Scan(); {
		line := sc.Text()
		if strings.Contains(line, "_version") {
			versionLine = strings.TrimSpace(line)
			break
		}
	}
	if versionLine == "" {
		return "", nil, errors.New("no version property")
	}

	match := reVersion.FindStringSubmatch(versionLine)
	if match == nil {
		return "", nil, fmt.Errorf("can not parse version property '%s'", versionLine)
	}

	server, version := match[1], match[2]
	ver, err := semver.New(version)
	if err != nil {
		return "", nil, err
	}

	return server, ver, nil
}
