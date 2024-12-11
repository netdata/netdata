// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type postqueueStats struct {
	sizeKbyte int64
	requests  int64
}

func (c *Collector) collect() (map[string]int64, error) {
	bs, err := c.exec.list()
	if err != nil {
		return nil, err
	}

	stats, err := parsePostqueueOutput(bs)
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	mx["emails"] = stats.requests
	mx["size"] = stats.sizeKbyte

	return mx, nil
}

func parsePostqueueOutput(bs []byte) (*postqueueStats, error) {
	if len(bs) == 0 {
		return nil, errors.New("empty postqueue output")
	}

	var lastLine string
	sc := bufio.NewScanner(bytes.NewReader(bs))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			lastLine = strings.TrimSpace(sc.Text())
		}
	}

	if lastLine == "Mail queue is empty" {
		return &postqueueStats{}, nil
	}

	// -- 3 Kbytes in 3 Requests.
	parts := strings.Fields(lastLine)
	if len(parts) < 5 {
		return nil, fmt.Errorf("unexpected postqueue output ('%s')", lastLine)
	}

	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("unexpected postqueue output ('%s')", lastLine)
	}
	requests, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("unexpected postqueue output ('%s')", lastLine)
	}

	return &postqueueStats{sizeKbyte: size, requests: requests}, nil
}
