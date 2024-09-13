// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("hdfs", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *HDFS {
	config := Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL: "http://127.0.0.1:9870/jmx",
			},
			ClientConfig: web.ClientConfig{
				Timeout: confopt.Duration(time.Second),
			},
		},
	}

	return &HDFS{
		Config: config,
	}
}

type Config struct {
	web.HTTPConfig `yaml:",inline" json:""`
	UpdateEvery    int `yaml:"update_every" json:"update_every"`
}

type (
	HDFS struct {
		module.Base
		Config `yaml:",inline" json:""`

		client *client

		nodeType
	}
	nodeType string
)

const (
	dataNodeType nodeType = "DataNode"
	nameNodeType nodeType = "NameNode"
)

func (h *HDFS) Configuration() any {
	return h.Config
}

func (h *HDFS) Init() error {
	if err := h.validateConfig(); err != nil {
		h.Errorf("config validation: %v", err)
		return err
	}

	cl, err := h.createClient()
	if err != nil {
		h.Errorf("error on creating client : %v", err)
		return err
	}
	h.client = cl

	return nil
}

func (h *HDFS) Check() error {
	typ, err := h.determineNodeType()
	if err != nil {
		h.Errorf("error on node type determination : %v", err)
		return err
	}
	h.nodeType = typ

	mx, err := h.collect()
	if err != nil {
		h.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (h *HDFS) Charts() *Charts {
	switch h.nodeType {
	default:
		return nil
	case nameNodeType:
		return nameNodeCharts()
	case dataNodeType:
		return dataNodeCharts()
	}
}

func (h *HDFS) Collect() map[string]int64 {
	mx, err := h.collect()

	if err != nil {
		h.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (h *HDFS) Cleanup() {
	if h.client != nil && h.client.httpClient != nil {
		h.client.httpClient.CloseIdleConnections()
	}
}
