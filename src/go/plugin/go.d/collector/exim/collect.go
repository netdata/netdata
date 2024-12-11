// SPDX-License-Identifier: GPL-3.0-or-later

package exim

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	resp, err := c.exec.countMessagesInQueue()
	if err != nil {
		return nil, err
	}

	emails, err := parseResponse(resp)
	if err != nil {
		return nil, err
	}

	mx := map[string]int64{
		"emails": emails,
	}

	return mx, nil
}

func parseResponse(resp []byte) (int64, error) {
	sc := bufio.NewScanner(bytes.NewReader(resp))
	sc.Scan()

	line := strings.TrimSpace(sc.Text())

	emails, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid response '%s': %v", line, err)
	}

	return emails, nil
}
