// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
)

// https://github.com/memcached/memcached/blob/b1aefcdf8a265f8a5126e8aa107a50988fa1ec35/doc/protocol.txt#L1267
var statsMetrics = map[string]bool{
	"limit_maxbytes":       true,
	"bytes":                true,
	"bytes_read":           true,
	"bytes_written":        true,
	"cas_badval":           true,
	"cas_hits":             true,
	"cas_misses":           true,
	"cmd_get":              true,
	"cmd_set":              true,
	"cmd_touch":            true,
	"curr_connections":     true,
	"curr_items":           true,
	"decr_hits":            true,
	"decr_misses":          true,
	"delete_hits":          true,
	"delete_misses":        true,
	"evictions":            true,
	"get_hits":             true,
	"get_misses":           true,
	"incr_hits":            true,
	"incr_misses":          true,
	"reclaimed":            true,
	"rejected_connections": true,
	"total_connections":    true,
	"total_items":          true,
	"touch_hits":           true,
	"touch_misses":         true,
}

func (c *Collector) collect() (map[string]int64, error) {
	if c.conn == nil {
		conn, err := c.establishConn()
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}

	stats, err := c.conn.queryStats()
	if err != nil {
		c.conn.disconnect()
		c.conn = nil
		return nil, err
	}

	mx := make(map[string]int64)

	if err := c.collectStats(mx, stats); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectStats(mx map[string]int64, stats []byte) error {
	if len(stats) == 0 {
		return errors.New("empty stats response")
	}

	var n int
	sc := bufio.NewScanner(bytes.NewReader(stats))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch {
		case strings.HasPrefix(line, "STAT"):
			key, value := getStatKeyValue(line)
			if !statsMetrics[key] {
				continue
			}
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				mx[key] = v
				n++
			}
		case strings.HasPrefix(line, "ERROR"):
			return errors.New("received ERROR response")
		}
	}

	if n == 0 {
		return errors.New("unexpected memcached response")
	}

	mx["avail"] = mx["limit_maxbytes"] - mx["bytes"]

	return nil
}

func (c *Collector) establishConn() (memcachedConn, error) {
	conn := c.newMemcachedConn(c.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}

func getStatKeyValue(line string) (string, string) {
	line = strings.TrimPrefix(line, "STAT ")
	i := strings.IndexByte(line, ' ')
	if i < 0 {
		return "", ""
	}
	return line[:i], line[i+1:]
}
