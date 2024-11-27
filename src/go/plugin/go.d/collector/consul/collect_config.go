// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"github.com/blang/semver/v4"
)

const (
	// https://developer.hashicorp.com/consul/api-docs/agent#read-configuration
	urlPathAgentSelf = "/v1/agent/self"
)

type consulConfig struct {
	Config struct {
		Datacenter        string
		PrimaryDatacenter string
		NodeName          string
		NodeID            string
		Server            bool
		Version           string
	}
	DebugConfig struct {
		Telemetry struct {
			MetricsPrefix   string
			DisableHostname bool
			PrometheusOpts  struct {
				Expiration string
				Name       string
			}
		}
		Cloud struct {
			AuthURL      string
			ClientID     string
			ClientSecret string
			Hostname     string
			ResourceID   string
			ScadaAddress string
		}
	}
	Stats struct {
		License struct {
			ID string `json:"id"`
		} `json:"license"`
	}
}

func (c *Collector) collectConfiguration() error {
	req, err := c.createRequest(urlPathAgentSelf)
	if err != nil {
		return err
	}

	var cfg consulConfig

	if err := c.client().RequestJSON(req, &cfg); err != nil {
		return err
	}

	c.cfg = &cfg
	c.Debugf("consul config: %+v", cfg)

	if !c.isTelemetryPrometheusEnabled() {
		c.Warning("export of Prometheus metrics is disabled")
	}

	ver, err := semver.New(c.cfg.Config.Version)
	if err != nil {
		c.Warningf("error on parsing Consul version '%s': %v", c.cfg.Config.Version, err)
		return nil
	}

	c.version = ver

	return nil
}
