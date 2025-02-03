package dnsquery

import (
	"os"

	"github.com/docker/docker/libnetwork/resolvconf"
)

func getResolvConfNameservers() ([]string, error) {
	path := resolvconf.Path()
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return resolvconf.GetNameservers(bs, resolvconf.IP), nil
}
