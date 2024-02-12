// SPDX-License-Identifier: GPL-3.0-or-later

package ntpd

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/iprange"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ntpd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *NTPd {
	return &NTPd{
		Config: Config{
			Address:      "127.0.0.1:123",
			Timeout:      web.Duration{Duration: time.Second * 3},
			CollectPeers: false,
		},
		charts:         systemCharts.Copy(),
		newClient:      newNTPClient,
		findPeersEvery: time.Minute * 3,
		peerAddr:       make(map[string]bool),
	}
}

type Config struct {
	Address      string       `yaml:"address"`
	Timeout      web.Duration `yaml:"timeout"`
	CollectPeers bool         `yaml:"collect_peers"`
}

type (
	NTPd struct {
		module.Base
		Config `yaml:",inline"`

		charts *module.Charts

		newClient func(c Config) (ntpConn, error)
		client    ntpConn

		findPeersTime    time.Time
		findPeersEvery   time.Duration
		peerAddr         map[string]bool
		peerIDs          []uint16
		peerIPAddrFilter iprange.Pool
	}
	ntpConn interface {
		systemInfo() (map[string]string, error)
		peerInfo(id uint16) (map[string]string, error)
		peerIDs() ([]uint16, error)
		close()
	}
)

func (n *NTPd) Init() bool {
	if n.Address == "" {
		n.Error("config validation: 'address' can not be empty")
		return false
	}

	txt := "0.0.0.0 127.0.0.0/8"
	r, err := iprange.ParseRanges(txt)
	if err != nil {
		n.Errorf("error on parse ip range '%s': %v", txt, err)
		return false
	}

	n.peerIPAddrFilter = r

	return true
}

func (n *NTPd) Check() bool {
	return len(n.Collect()) > 0
}

func (n *NTPd) Charts() *module.Charts {
	return n.charts
}

func (n *NTPd) Collect() map[string]int64 {
	mx, err := n.collect()
	if err != nil {
		n.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (n *NTPd) Cleanup() {
	if n.client != nil {
		n.client.close()
		n.client = nil
	}
}
