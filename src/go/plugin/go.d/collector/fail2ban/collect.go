// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (c *Collector) collect() (map[string]int64, error) {
	now := time.Now()

	if now.Sub(c.lastDiscoverTime) > c.discoverEvery || c.forceDiscover {
		jails, err := c.discoverJails()
		if err != nil {
			return nil, err
		}
		c.jails = jails
		c.lastDiscoverTime = now
		c.forceDiscover = false
	}

	mx := make(map[string]int64)

	if err := c.collectJails(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) discoverJails() ([]string, error) {
	bs, err := c.exec.status()
	if err != nil {
		return nil, err
	}

	jails, err := parseFail2banStatus(bs)
	if err != nil {
		return nil, err
	}

	if len(jails) == 0 {
		return nil, errors.New("no jails found")
	}

	c.Debugf("discovered %d jails: %v", len(jails), jails)

	return jails, nil
}

func (c *Collector) collectJails(mx map[string]int64) error {
	seen := make(map[string]bool)

	for _, jail := range c.jails {
		c.Debugf("querying status for jail '%s'", jail)
		bs, err := c.exec.jailStatus(jail)
		if err != nil {
			if errors.Is(err, errJailNotExist) {
				c.forceDiscover = true
				continue
			}
			return err
		}

		failed, banned, err := parseFail2banJailStatus(bs)
		if err != nil {
			return err
		}

		if !c.seenJails[jail] {
			c.seenJails[jail] = true
			c.addJailCharts(jail)
		}
		seen[jail] = true

		px := fmt.Sprintf("jail_%s_", jail)

		mx[px+"currently_failed"] = failed
		mx[px+"currently_banned"] = banned
	}

	for jail := range c.seenJails {
		if !seen[jail] {
			delete(c.seenJails, jail)
			c.removeJailCharts(jail)
		}
	}

	return nil
}

func parseFail2banJailStatus(jailStatus []byte) (failed, banned int64, err error) {
	const (
		failedSub = "Currently failed:"
		bannedSub = "Currently banned:"
	)

	var failedFound, bannedFound bool

	sc := bufio.NewScanner(bytes.NewReader(jailStatus))

	for sc.Scan() && !(failedFound && bannedFound) {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}

		if !failedFound {
			if i := strings.Index(text, failedSub); i != -1 {
				failedFound = true
				s := strings.TrimSpace(text[i+len(failedSub):])
				if failed, err = strconv.ParseInt(s, 10, 64); err != nil {
					return 0, 0, fmt.Errorf("failed to parse currently failed value (%s): %v", s, err)
				}
			}
		}
		if !bannedFound {
			if i := strings.Index(text, bannedSub); i != -1 {
				bannedFound = true
				s := strings.TrimSpace(text[i+len(bannedSub):])
				if banned, err = strconv.ParseInt(s, 10, 64); err != nil {
					return 0, 0, fmt.Errorf("failed to parse currently banned value (%s): %v", s, err)
				}
			}
		}
	}

	if !failedFound || !bannedFound {
		return 0, 0, errors.New("failed to find failed and banned values")
	}

	return failed, banned, nil
}

func parseFail2banStatus(status []byte) ([]string, error) {
	const sub = "Jail list:"

	var jails []string

	sc := bufio.NewScanner(bytes.NewReader(status))

	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())

		if i := strings.Index(text, sub); i != -1 {
			s := strings.ReplaceAll(text[i+len(sub):], ",", "")
			jails = strings.Fields(s)
			break
		}
	}

	if len(jails) == 0 {
		return nil, errors.New("failed to find jails")
	}

	return jails, nil
}
