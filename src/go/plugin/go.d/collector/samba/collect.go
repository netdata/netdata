// SPDX-License-Identifier: GPL-3.0-or-later

package samba

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	bs, err := c.exec.profile()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	if err := c.collectSmbStatusProfile(mx, bs); err != nil {
		return nil, err
	}

	c.once.Do(func() {
		c.addCharts(mx)
	})

	return mx, nil
}

func (c *Collector) collectSmbStatusProfile(mx map[string]int64, profileData []byte) error {
	sc := bufio.NewScanner(bytes.NewReader(profileData))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch {
		case strings.HasPrefix(line, "syscall_"):
		case strings.HasPrefix(line, "smb2_"):
		default:
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			c.Debugf("failed to parse line: '%s'", line)
			continue
		}

		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		if !(strings.HasSuffix(key, "count") || strings.HasSuffix(key, "bytes")) {
			continue
		}

		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			c.Debugf("failed to parse value in '%s': %v", line, err)
			continue
		}

		mx[key] = v
	}

	if len(mx) == 0 {
		return errors.New("unexpected smbstatus profile response: no metrics found")
	}

	return nil
}
