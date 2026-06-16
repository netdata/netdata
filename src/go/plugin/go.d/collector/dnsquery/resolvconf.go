package dnsquery

import (
	"bufio"
	"bytes"
	"net/netip"
	"os"
	"strings"
)

const (
	defaultResolvConfPath   = "/etc/resolv.conf"
	systemdResolvConfPath   = "/run/systemd/resolve/resolv.conf"
	systemdStubNameserverIP = "127.0.0.53"
)

func getResolvConfNameservers() ([]string, error) {
	path := resolvConfPath()
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseResolvConfNameservers(bs), nil
}

func resolvConfPath() string {
	bs, err := os.ReadFile(defaultResolvConfPath)
	if err != nil {
		return defaultResolvConfPath
	}
	nameservers := parseResolvConfNameservers(bs)
	if useSystemdResolvConf(nameservers) {
		return systemdResolvConfPath
	}
	return defaultResolvConfPath
}

func useSystemdResolvConf(nameservers []string) bool {
	return len(nameservers) == 1 && nameservers[0] == systemdStubNameserverIP
}

func parseResolvConfNameservers(bs []byte) []string {
	var nameservers []string
	sc := bufio.NewScanner(bytes.NewReader(bs))
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		addr, err := netip.ParseAddr(fields[1])
		if err != nil {
			continue
		}
		nameservers = append(nameservers, addr.String())
	}
	if err := sc.Err(); err != nil {
		return nil
	}
	return nameservers
}
