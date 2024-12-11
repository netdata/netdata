// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/miekg/dns"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.dnsClient == nil {
		c.dnsClient = c.newDNSClient(c.Network, c.Timeout.Duration())
	}

	mx := make(map[string]int64)
	domain := randomDomain(c.Domains)
	c.Debugf("current domain : %s", domain)

	var wg sync.WaitGroup
	var mux sync.RWMutex
	for _, srv := range c.Servers {
		for rtypeName, rtype := range c.recordTypes {
			wg.Add(1)
			go func(srv, rtypeName string, rtype uint16, wg *sync.WaitGroup) {
				defer wg.Done()

				msg := new(dns.Msg)
				msg.SetQuestion(dns.Fqdn(domain), rtype)
				address := net.JoinHostPort(srv, strconv.Itoa(c.Port))

				resp, rtt, err := c.dnsClient.Exchange(msg, address)

				mux.Lock()
				defer mux.Unlock()

				px := "server_" + srv + "_record_" + rtypeName + "_"

				mx[px+"query_status_success"] = 0
				mx[px+"query_status_network_error"] = 0
				mx[px+"query_status_dns_error"] = 0

				if err != nil {
					c.Debugf("error on querying %s after %s query for %s : %s", srv, rtypeName, domain, err)
					mx[px+"query_status_network_error"] = 1
					return
				}

				if resp != nil && resp.Rcode != dns.RcodeSuccess {
					c.Debugf("invalid answer from %s after %s query for %s (rcode %d)", srv, rtypeName, domain, resp.Rcode)
					mx[px+"query_status_dns_error"] = 1
				} else {
					mx[px+"query_status_success"] = 1
				}
				mx["server_"+srv+"_record_"+rtypeName+"_query_time"] = rtt.Nanoseconds()

			}(srv, rtypeName, rtype, &wg)
		}
	}
	wg.Wait()

	return mx, nil
}

func randomDomain(domains []string) string {
	src := rand.NewSource(time.Now().UnixNano())
	r := rand.New(src)
	return domains[r.Intn(len(domains))]
}
