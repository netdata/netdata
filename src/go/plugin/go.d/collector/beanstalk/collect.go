// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
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

	if err := c.collectStats(mx); err != nil {
		c.Cleanup(context.Background())
		return nil, err
	}
	if err := c.collectTubesStats(mx); err != nil {
		return mx, err
	}

	return mx, nil
}

func (c *Collector) collectStats(mx map[string]int64) error {
	stats, err := c.conn.queryStats()
	if err != nil {
		return err
	}
	for k, v := range stm.ToMap(stats) {
		mx[k] = v
	}
	return nil
}

func (c *Collector) collectTubesStats(mx map[string]int64) error {
	now := time.Now()

	if now.Sub(c.lastDiscoverTubesTime) > c.discoverTubesEvery {
		tubes, err := c.conn.queryListTubes()
		if err != nil {
			return err
		}

		c.Debugf("discovered tubes (%d): %v", len(tubes), tubes)
		v := slices.DeleteFunc(tubes, func(s string) bool { return !c.tubeSr.MatchString(s) })
		if len(tubes) != len(v) {
			c.Debugf("discovered tubes after filtering (%d): %v", len(v), v)
		}

		c.discoveredTubes = v
		c.lastDiscoverTubesTime = now
	}

	seen := make(map[string]bool)

	for i, tube := range c.discoveredTubes {
		if tube == "" {
			continue
		}

		stats, err := c.conn.queryStatsTube(tube)
		if err != nil {
			return err
		}

		if stats == nil {
			c.Infof("tube '%s' stats object not found (tube does not exist)", tube)
			c.discoveredTubes[i] = ""
			continue
		}
		if stats.Name == "" {
			c.Debugf("tube '%s' stats object has an empty name, ignoring it", tube)
			c.discoveredTubes[i] = ""
			continue
		}

		seen[stats.Name] = true
		if !c.seenTubes[stats.Name] {
			c.seenTubes[stats.Name] = true
			c.addTubeCharts(stats.Name)
		}

		px := fmt.Sprintf("tube_%s_", stats.Name)
		for k, v := range stm.ToMap(stats) {
			mx[px+k] = v
		}
	}

	for tube := range c.seenTubes {
		if !seen[tube] {
			delete(c.seenTubes, tube)
			c.removeTubeCharts(tube)
		}
	}

	return nil
}

func (c *Collector) establishConn() (beanstalkConn, error) {
	conn := c.newConn(c.Config, c.Logger)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
