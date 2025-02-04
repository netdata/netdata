// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/miekg/dns"
)

func (c *Collector) verifyConfig() error {
	if len(c.Domains) == 0 {
		return errors.New("no domains specified")
	}

	if !(c.Network == "" || c.Network == "udp" || c.Network == "tcp" || c.Network == "tcp-tls") {
		return fmt.Errorf("wrong network transport : %s", c.Network)
	}

	if c.RecordType != "" {
		c.Warning("'record_type' config option is deprecated, use 'record_types' instead")
		c.RecordTypes = append(c.RecordTypes, c.RecordType)
	}

	if len(c.RecordTypes) == 0 {
		return errors.New("no record types specified")
	}

	return nil
}

func (c *Collector) initServers() error {
	if len(c.Servers) != 0 {
		return nil
	}
	servers, err := getResolvConfNameservers()
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		return errors.New("no resolv conf nameservers")
	}

	c.Debugf("resolv conf nameservers: %v", servers)
	c.Servers = servers

	return nil
}

func (c *Collector) initRecordTypes() (map[string]uint16, error) {
	types := make(map[string]uint16)
	for _, v := range c.RecordTypes {
		rtype, err := parseRecordType(v)
		if err != nil {
			return nil, err
		}
		types[v] = rtype

	}

	return types, nil
}

func (c *Collector) initCharts() (*module.Charts, error) {
	charts := module.Charts{}

	for _, srv := range c.Servers {
		for _, rtype := range c.RecordTypes {
			cs := newDNSServerCharts(srv, c.Network, rtype)
			if err := charts.Add(*cs...); err != nil {
				return nil, err
			}
		}
	}

	return &charts, nil
}

func parseRecordType(recordType string) (uint16, error) {
	var rtype uint16

	switch recordType {
	case "A":
		rtype = dns.TypeA
	case "AAAA":
		rtype = dns.TypeAAAA
	case "ANY":
		rtype = dns.TypeANY
	case "CNAME":
		rtype = dns.TypeCNAME
	case "MX":
		rtype = dns.TypeMX
	case "NS":
		rtype = dns.TypeNS
	case "PTR":
		rtype = dns.TypePTR
	case "SOA":
		rtype = dns.TypeSOA
	case "SPF":
		rtype = dns.TypeSPF
	case "SRV":
		rtype = dns.TypeSRV
	case "TXT":
		rtype = dns.TypeTXT
	default:
		return 0, fmt.Errorf("unknown record type : %s", recordType)
	}

	return rtype, nil
}
