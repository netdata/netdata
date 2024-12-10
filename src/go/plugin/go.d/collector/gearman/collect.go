// SPDX-License-Identifier: GPL-3.0-or-later

package gearman

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.conn == nil {
		conn, err := c.establishConn()
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}

	status, err := c.conn.queryStatus()
	if err != nil {
		c.Cleanup(context.Background())
		return nil, fmt.Errorf("couldn't query status: %v", err)
	}

	prioStatus, err := c.conn.queryPriorityStatus()
	if err != nil {
		c.Cleanup(context.Background())
		return nil, fmt.Errorf("couldn't query priority status: %v", err)
	}

	mx := make(map[string]int64)

	if err := c.collectStatus(mx, status); err != nil {
		return nil, fmt.Errorf("couldn't collect status: %v", err)
	}
	if err := c.collectPriorityStatus(mx, prioStatus); err != nil {
		return nil, fmt.Errorf("couldn't collect priority status: %v", err)
	}

	return mx, nil

}

func (c *Collector) collectStatus(mx map[string]int64, statusData []byte) error {
	/*
		Same output as the "gearadmin --status" command:

		FUNCTION\tTOTAL\tRUNNING\tAVAILABLE_WORKERS

		E.g.:

		prefix generic_worker4 78      78      500
		generic_worker2 78      78      500
		generic_worker3 0       0       760
		generic_worker1 0       0       500
	*/

	seen := make(map[string]bool)
	var foundEnd bool
	sc := bufio.NewScanner(bytes.NewReader(statusData))

	mx["total_jobs_queued"] = 0
	mx["total_jobs_running"] = 0
	mx["total_jobs_waiting"] = 0
	mx["total_workers_avail"] = 0

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		if foundEnd = line == "."; foundEnd {
			break
		}

		parts := strings.Fields(line)

		// Gearman does not remove old tasks. We are only interested in tasks that have stats.
		if len(parts) < 4 {
			continue
		}

		name := strings.Join(parts[:len(parts)-3], "_")
		metrics := parts[len(parts)-3:]

		var queued, running, availWorkers int64
		var err error

		if queued, err = strconv.ParseInt(metrics[0], 10, 64); err != nil {
			return fmt.Errorf("couldn't parse queued count: %v", err)
		}
		if running, err = strconv.ParseInt(metrics[1], 10, 64); err != nil {
			return fmt.Errorf("couldn't parse running count: %v", err)
		}
		if availWorkers, err = strconv.ParseInt(metrics[2], 10, 64); err != nil {
			return fmt.Errorf("couldn't parse available count: %v", err)
		}

		px := fmt.Sprintf("function_%s_", name)

		waiting := queued - running

		mx[px+"jobs_queued"] = queued
		mx[px+"jobs_running"] = running
		mx[px+"jobs_waiting"] = waiting
		mx[px+"workers_available"] = availWorkers

		mx["total_jobs_queued"] += queued
		mx["total_jobs_running"] += running
		mx["total_jobs_waiting"] += waiting
		mx["total_workers_available"] += availWorkers

		seen[name] = true
	}

	if !foundEnd {
		return errors.New("unexpected status response")
	}

	for name := range seen {
		if !c.seenTasks[name] {
			c.seenTasks[name] = true
			c.addFunctionStatusCharts(name)
		}
	}
	for name := range c.seenTasks {
		if !seen[name] {
			delete(c.seenTasks, name)
			c.removeFunctionStatusCharts(name)
		}
	}

	return nil
}

func (c *Collector) collectPriorityStatus(mx map[string]int64, prioStatusData []byte) error {
	/*
		Same output as the "gearadmin --priority-status" command:

		FUNCTION\tHIGH\tNORMAL\tLOW\tAVAILABLE_WORKERS
	*/

	seen := make(map[string]bool)
	var foundEnd bool
	sc := bufio.NewScanner(bytes.NewReader(prioStatusData))

	mx["total_high_priority_jobs"] = 0
	mx["total_normal_priority_jobs"] = 0
	mx["total_low_priority_jobs"] = 0

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		if foundEnd = line == "."; foundEnd {
			break
		}

		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}

		name := strings.Join(parts[:len(parts)-4], "_")
		metrics := parts[len(parts)-4:]

		var high, normal, low int64
		var err error

		if high, err = strconv.ParseInt(metrics[0], 10, 64); err != nil {
			return fmt.Errorf("couldn't parse high count: %v", err)
		}
		if normal, err = strconv.ParseInt(metrics[1], 10, 64); err != nil {
			return fmt.Errorf("couldn't parse normal count: %v", err)
		}
		if low, err = strconv.ParseInt(metrics[2], 10, 64); err != nil {
			return fmt.Errorf("couldn't parse low count: %v", err)
		}

		px := fmt.Sprintf("function_%s_", name)

		mx[px+"high_priority_jobs"] = high
		mx[px+"normal_priority_jobs"] = normal
		mx[px+"low_priority_jobs"] = low
		mx["total_high_priority_jobs"] += high
		mx["total_normal_priority_jobs"] += normal
		mx["total_low_priority_jobs"] += low

		seen[name] = true
	}

	if !foundEnd {
		return errors.New("unexpected priority status response")
	}

	for name := range seen {
		if !c.seenPriorityTasks[name] {
			c.seenPriorityTasks[name] = true
			c.addFunctionPriorityStatusCharts(name)
		}
	}
	for name := range c.seenPriorityTasks {
		if !seen[name] {
			delete(c.seenPriorityTasks, name)
			c.removeFunctionPriorityStatusCharts(name)
		}
	}

	return nil
}

func (c *Collector) establishConn() (gearmanConn, error) {
	conn := c.newConn(c.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
