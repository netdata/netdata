// SPDX-License-Identifier: GPL-3.0-or-later

package tor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.conn == nil {
		conn, err := c.establishConnection()
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}

	mx := make(map[string]int64)
	if err := c.collectServerInfo(mx); err != nil {
		c.Cleanup(context.Background())
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectServerInfo(mx map[string]int64) error {
	resp, err := c.conn.getInfo("traffic/read", "traffic/written", "uptime")
	if err != nil {
		return err
	}

	sc := bufio.NewScanner(bytes.NewReader(resp))

	for sc.Scan() {
		line := sc.Text()

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("failed to parse metric: %s", line)
		}

		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse metric %s value: %v", line, err)
		}
		mx[key] = v
	}

	return nil
}

func (c *Collector) establishConnection() (controlConn, error) {
	conn := c.newConn(c.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
