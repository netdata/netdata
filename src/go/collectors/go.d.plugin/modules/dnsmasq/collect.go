// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

func (d *Dnsmasq) collect() (map[string]int64, error) {
	r, err := d.queryCacheStatistics()
	if err != nil {
		return nil, err
	}

	ms := make(map[string]int64)
	if err = d.collectResponse(ms, r); err != nil {
		return nil, err
	}

	return ms, nil
}

func (d *Dnsmasq) collectResponse(ms map[string]int64, resp *dns.Msg) error {
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
	for _, a := range resp.Answer {
		txt, ok := a.(*dns.TXT)
		if !ok {
			continue
		}

		idx := strings.IndexByte(txt.Hdr.Name, '.')
		if idx == -1 {
			continue
		}

		switch name := txt.Hdr.Name[:idx]; name {
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

				ms["queries"] += int64(queries)
				ms["failed_queries"] += int64(failedQueries)
			}
		case "cachesize", "insertions", "evictions", "hits", "misses", "auth":
			if len(txt.Txt) != 1 {
				return fmt.Errorf("parse '%s' (%v): unexpected format", txt.Hdr.Name, txt.Txt)
			}
			v, err := strconv.ParseFloat(txt.Txt[0], 64)
			if err != nil {
				return fmt.Errorf("parse '%s' (%s): %v", txt.Hdr.Name, txt.Txt[0], err)
			}

			ms[name] = int64(v)
		}
	}
	return nil
}

func (d *Dnsmasq) queryCacheStatistics() (*dns.Msg, error) {
	msg := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               dns.Id(),
			RecursionDesired: true,
		},
		Question: []dns.Question{
			{Name: "cachesize.bind.", Qtype: dns.TypeTXT, Qclass: dns.ClassCHAOS},
			{Name: "insertions.bind.", Qtype: dns.TypeTXT, Qclass: dns.ClassCHAOS},
			{Name: "evictions.bind.", Qtype: dns.TypeTXT, Qclass: dns.ClassCHAOS},
			{Name: "hits.bind.", Qtype: dns.TypeTXT, Qclass: dns.ClassCHAOS},
			{Name: "misses.bind.", Qtype: dns.TypeTXT, Qclass: dns.ClassCHAOS},
			// TODO: collect auth.bind if available
			// auth.bind query is only supported if dnsmasq has been built
			// to support running as an authoritative name server. See https://github.com/netdata/netdata/issues/13766
			//{Name: "auth.bind.", Qtype: dns.TypeTXT, Qclass: dns.ClassCHAOS},
			{Name: "servers.bind.", Qtype: dns.TypeTXT, Qclass: dns.ClassCHAOS},
		},
	}

	r, _, err := d.dnsClient.Exchange(msg, d.Address)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("'%s' returned an empty response", d.Address)
	}
	if r.Rcode != dns.RcodeSuccess {
		s := dns.RcodeToString[r.Rcode]
		return nil, fmt.Errorf("'%s' returned '%s' (%d) response code", d.Address, s, r.Rcode)
	}
	return r, nil
}
