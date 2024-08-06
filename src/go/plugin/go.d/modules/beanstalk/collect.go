// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"fmt"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func (b *Beanstalk) collect() (map[string]int64, error) {
	if b.conn == nil {
		conn, err := b.establishConn()
		if err != nil {
			return nil, err
		}
		b.conn = conn
	}

	mx := make(map[string]int64)

	if err := b.collectStats(mx); err != nil {
		b.Cleanup()
		return nil, err
	}
	if err := b.collectTubesStats(mx); err != nil {
		return mx, err
	}

	return mx, nil
}

func (b *Beanstalk) collectStats(mx map[string]int64) error {
	stats, err := b.conn.queryStats()
	if err != nil {
		return err
	}
	for k, v := range stm.ToMap(stats) {
		mx[k] = v
	}
	return nil
}

func (b *Beanstalk) collectTubesStats(mx map[string]int64) error {
	now := time.Now()

	if now.Sub(b.lastDiscoverTubesTime) > b.discoverTubesEvery {
		tubes, err := b.conn.queryListTubes()
		if err != nil {
			return err
		}

		b.Debugf("discovered tubes (%d): %v", len(tubes), tubes)
		v := slices.DeleteFunc(tubes, func(s string) bool { return !b.tubeSr.MatchString(s) })
		if len(tubes) != len(v) {
			b.Debugf("discovered tubes after filtering (%d): %v", len(v), v)
		}

		b.discoveredTubes = v
		b.lastDiscoverTubesTime = now
	}

	seen := make(map[string]bool)

	for i, tube := range b.discoveredTubes {
		if tube == "" {
			continue
		}

		stats, err := b.conn.queryStatsTube(tube)
		if err != nil {
			return err
		}

		if stats == nil {
			b.Debugf("tube '%s' stats object not found (tube does not exist)", tube)
			b.discoveredTubes[i] = ""
			continue
		}
		if stats.Name == "" {
			b.Debugf("tube '%s' stats object has an empty name, ignoring it", tube)
			b.discoveredTubes[i] = ""
			continue
		}

		seen[stats.Name] = true
		if !b.seenTubes[stats.Name] {
			b.seenTubes[stats.Name] = true
			b.addTubeCharts(stats.Name)
		}

		px := fmt.Sprintf("tube_%s_", stats.Name)
		for k, v := range stm.ToMap(stats) {
			mx[px+k] = v
		}
	}

	for tube := range b.seenTubes {
		if !seen[tube] {
			delete(b.seenTubes, tube)
			b.removeTubeCharts(tube)
		}
	}

	return nil
}

func (b *Beanstalk) establishConn() (beanstalkConn, error) {
	conn := b.newConn(b.Config, b.Logger)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
