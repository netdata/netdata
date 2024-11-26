// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectCacheStatistics(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectCacheStatistics(mx map[string]int64) error {
	/*
		;; flags: qr aa rd ra; QUERY: 7, ANSWER: 7, AUTHORITY: 0, ADDITIONAL: 0

		;; QUESTION SECTION:
		;cachesize.bind.	CH	 TXT
		;insertions.bind.	CH	 TXT
		;evictions.bind.	CH	 TXT
		;hits.bind.			CH	 TXT
		;misses.bind.		CH	 TXT
		;auth.bind.			CH	 TXT
		;servers.bind.		CH	 TXT

		;; ANSWER SECTION:
		cachesize.bind.		0	CH	TXT	"150"
		insertions.bind.	0	CH	TXT	"1"
		evictions.bind.		0	CH	TXT	"0"
		hits.bind.			0	CH	TXT	"176"
		misses.bind.		0	CH	TXT	"4"
		auth.bind.			0	CH	TXT	"0"
		servers.bind.		0	CH	TXT	"10.0.0.1#53 0 0" "1.1.1.1#53 4 3" "1.0.0.1#53 3 0"
	*/

	questions := []string{
		"servers.bind.",
		"cachesize.bind.",
		"insertions.bind.",
		"evictions.bind.",
		"hits.bind.",
		"misses.bind.",
		// auth.bind query is only supported if dnsmasq has been built to support running as an authoritative name server
		// See https://github.com/netdata/netdata/issues/13766
		//"auth.bind.",
	}

	for _, q := range questions {
		resp, err := c.query(q)
		if err != nil {
			return err
		}

		for _, a := range resp.Answer {
			txt, ok := a.(*dns.TXT)
			if !ok {
				continue
			}

			idx := strings.IndexByte(txt.Hdr.Name, '.')
			if idx == -1 {
				continue
			}

			name := txt.Hdr.Name[:idx]

			switch name {
			case "servers":
				for _, entry := range txt.Txt {
					parts := strings.Fields(entry)
					if len(parts) != 3 {
						return fmt.Errorf("parse %s (%s): unexpected format", txt.Hdr.Name, entry)
					}
					queries, err := strconv.ParseFloat(parts[1], 64)
					if err != nil {
						return fmt.Errorf("parse '%s' (%s): %v", txt.Hdr.Name, entry, err)
					}
					failedQueries, err := strconv.ParseFloat(parts[2], 64)
					if err != nil {
						return fmt.Errorf("parse '%s' (%s): %v", txt.Hdr.Name, entry, err)
					}

					mx["queries"] += int64(queries)
					mx["failed_queries"] += int64(failedQueries)
				}
			case "cachesize", "insertions", "evictions", "hits", "misses", "auth":
				if len(txt.Txt) != 1 {
					return fmt.Errorf("parse '%s' (%v): unexpected format", txt.Hdr.Name, txt.Txt)
				}
				v, err := strconv.ParseFloat(txt.Txt[0], 64)
				if err != nil {
					return fmt.Errorf("parse '%s' (%s): %v", txt.Hdr.Name, txt.Txt[0], err)
				}

				mx[name] = int64(v)
			}
		}
	}

	return nil
}

func (c *Collector) query(question string) (*dns.Msg, error) {
	msg := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               dns.Id(),
			RecursionDesired: true,
		},
		Question: []dns.Question{
			{Name: question, Qtype: dns.TypeTXT, Qclass: dns.ClassCHAOS},
		},
	}

	r, _, err := c.dnsClient.Exchange(msg, c.Address)
	if err != nil {
		return nil, err
	}

	if r == nil {
		return nil, fmt.Errorf("'%s' question '%s', returned an empty response", c.Address, question)
	}

	if r.Rcode != dns.RcodeSuccess {
		s := dns.RcodeToString[r.Rcode]
		return nil, fmt.Errorf("'%s' question '%s' returned '%s' (%d) response code", c.Address, question, s, r.Rcode)
	}

	return r, nil
}
