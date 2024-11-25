// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package isc_dhcpd

import (
	"bufio"
	"bytes"
	"net"
	"os"
)

/*
Documentation (v4.4): https://kb.isc.org/docs/en/isc-dhcp-44-manual-pages-dhcpdleases

DHCPv4 prepare declaration:
  prepare ip-address {
    statements...
  }

DHCPv6 prepare declaration:
  ia_ta IAID_DUID {
    cltt date;
    iaaddr ipv6-address {
      statements...
    }
  }
  ia_na IAID_DUID {
    cltt date;
    iaaddr ipv6-address {
      statements...
    }
  }
  ia_pd IAID_DUID {
    cltt date;
    iaprefix ipv6-address/prefix-length {
      statements...
    }
  }
*/

type leaseEntry struct {
	ip           net.IP
	bindingState string
}

func (l leaseEntry) hasIP() bool           { return l.ip != nil }
func (l leaseEntry) hasBindingState() bool { return l.bindingState != "" }

func parseDHCPdLeasesFile(filepath string) ([]leaseEntry, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	leasesSet := make(map[string]leaseEntry)
	l := leaseEntry{}
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		bs := bytes.TrimSpace(sc.Bytes())
		switch {
		case !l.hasIP() && bytes.HasPrefix(bs, []byte("lease")):
			// "lease 192.168.0.1 {" => "192.168.0.1"
			s := string(bs)
			l.ip = net.ParseIP(s[6 : len(s)-2])
		case !l.hasIP() && bytes.HasPrefix(bs, []byte("iaaddr")):
			// "iaaddr 1985:470:1f0b:c9a::001 {" =>  "1985:470:1f0b:c9a::001"
			s := string(bs)
			l.ip = net.ParseIP(s[7 : len(s)-2])
		case l.hasIP() && !l.hasBindingState() && bytes.HasPrefix(bs, []byte("binding state")):
			// "binding state active;" => "active"
			s := string(bs)
			l.bindingState = s[14 : len(s)-1]
		case bytes.HasPrefix(bs, []byte("}")):
			if l.hasIP() && l.hasBindingState() {
				leasesSet[l.ip.String()] = l
			}
			l = leaseEntry{}
		}
	}

	if len(leasesSet) == 0 {
		return nil, nil
	}

	leases := make([]leaseEntry, 0, len(leasesSet))
	for _, l := range leasesSet {
		leases = append(leases, l)
	}
	return leases, nil
}
