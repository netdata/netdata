// SPDX-License-Identifier: GPL-3.0-or-later

package boinc

import (
	"fmt"
	"net"
)

func (b *Boinc) collect() (map[string]int64, error) {
	if b.conn == nil {
		conn, err := b.establishConn()
		if err != nil {
			return nil, err
		}
		b.conn = conn
	}

	results, err := b.conn.getResults()
	if err != nil {
		b.Cleanup()
		return nil, err
	}

	mx := make(map[string]int64)

	if err := b.collectResults(mx, results); err != nil {
		return nil, err
	}

	return mx, nil
}

func (b *Boinc) collectResults(mx map[string]int64, results []boincReplyResult) error {
	mx["total"] = int64(len(results))
	mx["active"] = 0

	for _, v := range resultStateMap {
		mx[v] = 0
	}
	for _, v := range activeTaskStateMap {
		mx[v] = 0
	}
	for _, v := range schedulerStateMap {
		mx[v] = 0
	}

	for _, r := range results {
		mx[r.state()]++
		if r.ActiveTask != nil {
			mx["active"]++
			mx[r.activeTaskState()]++
			mx[r.schedulerState()]++
		}
	}

	return nil
}

func (b *Boinc) establishConn() (boincConn, error) {
	conn := b.newConn(b.Config, b.Logger)

	if err := conn.connect(); err != nil {
		return nil, fmt.Errorf("failed to establish connection: %w", err)
	}

	if host, _, err := net.SplitHostPort(b.Address); err == nil {
		// for the commands we use, authentication is only required for remote connections
		ip := net.ParseIP(host)
		if host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return conn, nil
		}
	}

	if err := conn.authenticate(); err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	return conn, nil
}
