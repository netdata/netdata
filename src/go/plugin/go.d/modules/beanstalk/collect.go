// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// https://github.com/beanstalkd/beanstalkd/blob/master/doc/protocol.txt#L553C1-L561C15
var statsMetrics = map[string]bool{
	"rusage-utime":            true,
	"rusage-stime":            true,
	"total-jobs":              true,
	"job-timeouts":            true,
	"total-connections":       true,
	"cmd-put":                 true,
	"cmd-peek":                true,
	"cmd-peek-ready":          true,
	"cmd-peek-delayed":        true,
	"cmd-peek-buried":         true,
	"cmd-reserve":             true,
	"cmd-use":                 true,
	"cmd-watch":               true,
	"cmd-ignore":              true,
	"cmd-delete":              true,
	"cmd-release":             true,
	"cmd-bury":                true,
	"cmd-kick":                true,
	"cmd-stats":               true,
	"cmd-stats-job":           true,
	"cmd-stats-tube":          true,
	"cmd-list-tubes":          true,
	"cmd-list-tube-used":      true,
	"cmd-list-tubes-watched":  true,
	"cmd-pause-tube":          true,
	"current-tubes":           true,
	"current-jobs-urgent":     true,
	"current-jobs-ready":      true,
	"current-jobs-reserved":   true,
	"current-jobs-delayed":    true,
	"current-jobs-buried":     true,
	"current-connections":     true,
	"current-producers":       true,
	"current-workers":         true,
	"current-waiting":         true,
	"binlog-records-written":  true,
	"binlog-records-migrated": true,
	"uptime":                  true,
}

// https://github.com/beanstalkd/beanstalkd/blob/master/doc/protocol.txt#L515-L516
var tubeMetrics = map[string]bool{
	"total-jobs":            true,
	"current-jobs-urgent":   true,
	"current-jobs-ready":    true,
	"current-jobs-reserved": true,
	"current-jobs-delayed":  true,
	"current-jobs-buried":   true,
	"current-using":         true,
	"current-waiting":       true,
	"current-watching":      true,
	"cmd-delete":            true,
	"cmd-pause-tube":        true,
	"pause":                 true,
	"pause-time-left":       true,
}

func (b *Beanstalk) collect() (map[string]int64, error) {
	if b.conn == nil {
		conn, err := b.establishConn()
		if err != nil {
			return nil, err
		}
		b.conn = conn
	}

	stats, err := b.conn.queryStats()
	if err != nil {
		b.conn.disconnect()
		b.conn = nil
		return nil, err
	}

	data, err := b.conn.listTubes()
	if err != nil {
		b.conn.disconnect()
		b.conn = nil
		return nil, err
	}

	tubes, err := parseListTubes(data)
	if err != nil {
		b.conn.disconnect()
		b.conn = nil
		return nil, err
	}

	seenTubes := make(map[string]bool)
	mx := make(map[string]int64)

	for _, tube := range tubes {

		b.Debug("TUBE ", tube)

		seenTubes[tube] = true

		if !b.tubes[tube] {
			b.addTubeCharts(tube)
			b.tubes[tube] = true
		}

		statsTube, err := b.conn.statsTube(tube)
		if err != nil {
			b.conn.disconnect()
			b.conn = nil
			return nil, err
		}

		if err := b.collectTubeStats(mx, statsTube, tube); err != nil {
			return nil, err
		}

	}

	for _, tube := range tubes {
		if !seenTubes[tube] {
			b.removeTubeCharts(tube)
			delete(b.tubes, tube)
		}
	}

	if err := b.collectStats(mx, stats); err != nil {
		return nil, err
	}

	if !b.once {
		b.addBaseCharts()
		b.once = true
	}

	return mx, nil
}

func (b *Beanstalk) collectStats(mx map[string]int64, stats []byte) error {
	if len(stats) == 0 {
		return errors.New("empty stats response")
	}

	var n int
	sc := bufio.NewScanner(bytes.NewReader(stats))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch {
		case strings.HasPrefix(line, "OK"):
			b.Debug("response received, starting parsing")
		case strings.HasPrefix(line, "---"):
			b.Debug("dashline")
		case strings.HasPrefix(line, "UNKNOWN_COMMAND") || strings.HasPrefix(line, "NOT_FOUND"):
			return errors.New("received error response")
		case len(line) > 0:

			key, value := getStatKeyValue(line)
			if !statsMetrics[key] {
				continue
			}

			if floatValue, ok := parseFloat(value); ok {
				if math.Mod(floatValue, 1.0) == 0 {
					mx[key] = *parseInt(value)
					n++
				} else {

					mx[key] = int64(floatValue * 1000)
					n++

				}

			}

		}

	}
	return nil
}

func (b *Beanstalk) collectTubeStats(mx map[string]int64, stats []byte, tubeName string) error {
	if len(stats) == 0 {
		return errors.New("empty stats-tube response")
	}

	var n int
	sc := bufio.NewScanner(bytes.NewReader(stats))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch {
		case strings.HasPrefix(line, "OK"):
			b.Debug("response received, starting parsing")
		case strings.HasPrefix(line, "---"):
			b.Debug("dashline")
		case strings.HasPrefix(line, "UNKNOWN_COMMAND") || strings.HasPrefix(line, "NOT_FOUND"):
			return errors.New("received error response")
		case len(line) > 0:

			key, value := getStatKeyValue(line)
			if !tubeMetrics[key] {
				continue
			}

			if floatValue, ok := parseFloat(value); ok {
				if math.Mod(floatValue, 1.0) == 0 {
					mx[tubeName+"_"+key] = *parseInt(value)
					n++
				} else {

					mx[tubeName+"_"+key] = int64(floatValue * 1000)
					n++

				}

			}

		}

	}
	return nil
}

func (b *Beanstalk) establishConn() (beanstalkConn, error) {
	conn := b.newBeanstalkConn(b.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}

func getStatKeyValue(line string) (string, string) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", ""
	}

	key := line[:i]
	value := line[i+1:]

	return strings.TrimSpace(key), strings.TrimSpace(value)
}
func parseInt(value string) *int64 {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}
func parseFloat(s string) (float64, bool) {
	if s == "-" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func parseListTubes(data []byte) ([]string, error) {
	dataString := strings.TrimSpace(string(data))
	lines := strings.Split(dataString, "\n")
	if len(lines) < 2 || lines[0] != "---" {
		return nil, fmt.Errorf("unexpected data format" + lines[0])
	}

	tubes := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		tubes = append(tubes, strings.TrimSpace(line)[2:])
	}

	return tubes, nil
}
