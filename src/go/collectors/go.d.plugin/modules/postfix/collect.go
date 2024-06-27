// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strconv"
)

type postqueueStats struct {
	kbytes   int64
	requests int64
}

func (p *Postfix) collect() (map[string]int64, error) {
	bs, err := p.exec.list()
	if err != nil {
		return nil, err
	}

	stats, err := parsePostfixOutput(bs)
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	p.collectPostqueueStats(mx, *stats)

	return mx, nil
}

func (p *Postfix) collectPostqueueStats(mx map[string]int64, stats postqueueStats) {

	if !p.seen_metrics {
		p.addPostfixCharts()
		p.seen_metrics = true
	}

	mx["emails"] = stats.requests
	mx["size"] = stats.kbytes

}

func parsePostfixOutput(bs []byte) (*postqueueStats, error) {
	if len(bs) == 0 {
		return nil, fmt.Errorf("error: No bytes to read")
	}

	/*
		$ postqueue -p
			752741009D2A*   10438 Wed Jun 26 13:39:26  root@localhost.test
											fotis@localhost.test
		6B5FA10033D4*   10438 Wed Jun 26 13:39:23  root@localhost.test
											fotis@localhost.test
		-- 132422 Kbytes in 12991 Requests.
	*/

	sc := bufio.NewScanner(bytes.NewReader(bs))

	kbytes := int64(-1)
	requests := int64(-1)
	re := regexp.MustCompile(`-- (\d+) Kbytes in (\d+) Requests\.`)

	for sc.Scan() {
		line := sc.Text()
		if line == "Mail queue is empty" {
			kbytes = 0
			requests = 0
			break
		}

		matches := re.FindStringSubmatch(line)
		if matches != nil {
			kbytes, _ = strconv.ParseInt(matches[1], 10, 64)
			requests, _ = strconv.ParseInt(matches[2], 10, 64)
			break
		}
	}

	if err := sc.Err(); err != nil {
		log.Fatalf("Error reading output: %v", err)
	}

	if kbytes == -1 && requests == -1 {
		return nil, fmt.Errorf("unexpected response")
	}

	return &postqueueStats{
		kbytes:   kbytes,
		requests: requests,
	}, nil
}
