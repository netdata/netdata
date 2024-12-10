// SPDX-License-Identifier: GPL-3.0-or-later

package spigotmc

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const precision = 100

var (
	reTPS       = regexp.MustCompile(`(?ms)(?P<tps_1min>\d+.\d+),.*?(?P<tps_5min>\d+.\d+),.*?(?P<tps_15min>\d+.\d+).*?$.*?(?P<mem_used>\d+)/(?P<mem_alloc>\d+)[^:]+:\s*(?P<mem_max>\d+)`)
	reList      = regexp.MustCompile(`(?P<players>\d+)/?(?P<hidden_players>\d+)?.*?(?P<total_players>\d+)`)
	reCleanResp = regexp.MustCompile(`§.`)
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.conn == nil {
		conn, err := c.establishConn()
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}

	mx := make(map[string]int64)

	if err := c.collectTPS(mx); err != nil {
		c.Cleanup(context.Background())
		return nil, fmt.Errorf("failed to collect '%s': %v", cmdTPS, err)
	}
	if err := c.collectList(mx); err != nil {
		c.Cleanup(context.Background())
		return nil, fmt.Errorf("failed to collect '%s': %v", cmdList, err)
	}

	return mx, nil
}

func (c *Collector) collectTPS(mx map[string]int64) error {
	resp, err := c.conn.queryTps()
	if err != nil {
		return err
	}

	c.Debugf("cmd '%s' response: %s", cmdTPS, resp)

	if err := parseResponse(resp, reTPS, func(s string, f float64) {
		switch {
		case strings.HasPrefix(s, "tps"):
			f *= precision
		case strings.HasPrefix(s, "mem"):
			f *= 1024 * 1024 // mb to bytes
		}
		mx[s] = int64(f)
	}); err != nil {
		return err
	}

	return nil
}

func (c *Collector) collectList(mx map[string]int64) error {
	resp, err := c.conn.queryList()
	if err != nil {
		return err
	}
	c.Debugf("cmd '%s' response: %s", cmdList, resp)

	var players int64
	if err := parseResponse(resp, reList, func(s string, f float64) {
		switch s {
		case "players", "hidden_players":
			players += int64(f)
		}
	}); err != nil {
		return err
	}

	mx["players"] = players

	return nil
}

func parseResponse(resp string, re *regexp.Regexp, fn func(string, float64)) error {
	if resp == "" {
		return errors.New("empty response")
	}

	resp = reCleanResp.ReplaceAllString(resp, "")

	matches := re.FindStringSubmatch(resp)
	if len(matches) == 0 {
		return errors.New("regexp does not match")
	}

	for i, name := range re.SubexpNames() {
		if name == "" || len(matches) <= i || matches[i] == "" {
			continue
		}
		val := matches[i]

		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("failed to parse key '%s' value '%s': %v", name, val, err)
		}

		fn(name, v)
	}

	return nil
}

func (c *Collector) establishConn() (rconConn, error) {
	conn := c.newConn(c.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
