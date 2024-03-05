// SPDX-License-Identifier: GPL-3.0-or-later

package ntpd

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/iprange"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
			Timeout:      web.Duration(time.Second),
			CollectPeers: false,
		},
		charts:         systemCharts.Copy(),
		newClient:      newNTPClient,
		findPeersEvery: time.Minute * 3,
		peerAddr:       make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery  int          `yaml:"update_every" json:"update_every"`
	Address      string       `yaml:"address" json:"address"`
	Timeout      web.Duration `yaml:"timeout" json:"timeout"`
	CollectPeers bool         `yaml:"collect_peers" json:"collect_peers"`
}

type (
	NTPd struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client    ntpConn
		newClient func(c Config) (ntpConn, error)

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

func (n *NTPd) Configuration() any {
	return n.Config
}

func (n *NTPd) Init() error {
	if n.Address == "" {
		n.Error("config validation: 'address' can not be empty")
		return errors.New("address not set")
	}

	txt := "0.0.0.0 127.0.0.0/8"
	r, err := iprange.ParseRanges(txt)
	if err != nil {
		n.Errorf("error on parsing ip range '%s': %v", txt, err)
		return fmt.Errorf("error on parsing ip range '%s': %v", txt, err)
	}

	n.peerIPAddrFilter = r

	return nil
}

func (n *NTPd) Check() error {
	mx, err := n.collect()
	if err != nil {
		n.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
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
