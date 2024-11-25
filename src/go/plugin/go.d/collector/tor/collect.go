// SPDX-License-Identifier: GPL-3.0-or-later

package tor

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

func (t *Tor) collect() (map[string]int64, error) {
	if t.conn == nil {
		conn, err := t.establishConnection()
		if err != nil {
			return nil, err
		}
		t.conn = conn
	}

	mx := make(map[string]int64)
	if err := t.collectServerInfo(mx); err != nil {
		t.Cleanup()
		return nil, err
	}

	return mx, nil
}

func (t *Tor) collectServerInfo(mx map[string]int64) error {
	resp, err := t.conn.getInfo("traffic/read", "traffic/written", "uptime")
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

func (t *Tor) establishConnection() (controlConn, error) {
	conn := t.newConn(t.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
