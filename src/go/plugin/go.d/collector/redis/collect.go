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

	switch c.server {
	// redis and redis protocol-compatible servers (kvrocks, garnet); identified
	// by the first *_version line of the INFO response, or by the server's own
	// *_version property wherever it appears (garnet, kvrocks).
	case "redis", "kvrocks", "garnet":
	default:
		return nil, fmt.Errorf("unsupported server app, want one of: redis, kvrocks, garnet, got=%s", c.server)
	}

	if c.server == "garnet" {
		info = c.collectGarnetExtraInfo(info)
	}

	mx := make(map[string]int64)
	c.collectInfo(mx, info)
	c.collectPingLatency(mx)

	return mx, nil
}

// redis_version:6.0.9
var reVersion = regexp.MustCompile(`([a-z]+)_version:(\d+\.\d+\.\d+)`)

// extractServerVersion identifies the server app and its version from the
// INFO response. By default the first *_version property decides (Valkey,
// Dragonfly and KeyDB emit redis_version first and are served by the redis
// collector). Garnet and Kvrocks also emit a redis_version compatibility
// value, but always emit their own <server>_version property as well, which
// identifies both the server and its own version wherever it appears.
func extractServerVersion(info string) (string, *semver.Version, error) {
	versionLine := firstVersionLine(info, "")
	for _, server := range []string{"garnet", "kvrocks"} {
		if own := firstVersionLine(info, server+"_version"); own != "" {
			versionLine = own
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

// firstVersionLine returns the first INFO line naming a version property. With
// a non-empty field it returns the first line naming exactly that property.
func firstVersionLine(info, field string) string {
	want := "_version"
	if field != "" {
		want = field + ":"
	}
	for sc := bufio.NewScanner(strings.NewReader(info)); sc.Scan(); {
		if line := sc.Text(); strings.Contains(line, want) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
