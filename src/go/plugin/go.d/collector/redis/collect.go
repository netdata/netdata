// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const precision = 1000 // float values multiplier and dimensions divisor

// supportedServers are the server apps extractServerVersion can report that the collector accepts.
var supportedServers = []string{"redis", "kvrocks", "garnet"}

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	info, err := c.rdb.Info(ctx, "all").Result()
	if err != nil {
		return nil, err
	}

	if c.server == "" {
		server, version, err := extractServerVersion(info)
		if err != nil {
			return nil, fmt.Errorf("can not extract server app and version: %v", err)
		}
		c.server = server
		c.Debugf(`server="%s",version="%s"`, server, version)
	}

	if !slices.Contains(supportedServers, c.server) {
		return nil, fmt.Errorf("unsupported server app, want one of: %s, got=%s",
			strings.Join(supportedServers, ", "), c.server)
	}

	if c.server == "garnet" {
		info = c.appendGarnetInfoSections(ctx, info)
	}

	mx := make(map[string]int64)
	c.collectInfo(mx, info)
	c.collectPingLatency(ctx, mx)

	return mx, nil
}

// ownVersionFields are emitted by servers that also report a redis_version compatibility value. Each
// identifies its server and that server's own version wherever it appears in the INFO response.
var ownVersionFields = []string{"garnet_version", "kvrocks_version"}

// redis_version:6.0.9
var reVersion = regexp.MustCompile(`([a-z]+)_version:(\d+\.\d+\.\d+)`)

// extractServerVersion identifies the server app and its version from the INFO response. The first
// *_version property decides (Valkey, Dragonfly and KeyDB emit redis_version first and are served as
// redis), unless the response carries one of ownVersionFields.
func extractServerVersion(info string) (server, version string, err error) {
	var versionLine string
	for line := range strings.Lines(info) {
		line = strings.TrimSpace(line)
		if field, _, _ := strings.Cut(line, ":"); slices.Contains(ownVersionFields, field) {
			versionLine = line
			break
		}
		if versionLine == "" && strings.Contains(line, "_version") {
			versionLine = line
		}
	}
	if versionLine == "" {
		return "", "", errors.New("no version property")
	}

	match := reVersion.FindStringSubmatch(versionLine)
	if match == nil {
		return "", "", fmt.Errorf("can not parse version property '%s'", versionLine)
	}

	return match[1], match[2], nil
}
