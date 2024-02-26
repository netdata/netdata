package dyncfg

import (
	"errors"
	"os"
	"strings"

	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"

	"gopkg.in/yaml.v2"
)

func (d *Discovery) Register(cfg confgroup.Config) {
	name := cfgJobName(cfg)
	if cfg.Provider() != dynCfg {
		// jobType handling in ND is not documented
		_ = d.API.DynCfgRegisterJob(cfg.Module(), name, "stock")
	}

	key := cfg.Module() + "_" + name
	d.addConfig(key, cfg)
}

func (d *Discovery) Unregister(cfg confgroup.Config) {
	key := cfg.Module() + "_" + cfgJobName(cfg)
	d.removeConfig(key)
}

func (d *Discovery) UpdateStatus(cfg confgroup.Config, status, payload string) {
	_ = d.API.DynCfgReportJobStatus(cfg.Module(), cfgJobName(cfg), status, payload)
}

func (d *Discovery) addConfig(name string, cfg confgroup.Config) {
	d.mux.Lock()
	defer d.mux.Unlock()

	d.configs[name] = cfg
}

func (d *Discovery) removeConfig(key string) {
	d.mux.Lock()
	defer d.mux.Unlock()

	delete(d.configs, key)
}

func (d *Discovery) getConfig(key string) (confgroup.Config, bool) {
	d.mux.Lock()
	defer d.mux.Unlock()

	v, ok := d.configs[key]
	return v, ok
}

func (d *Discovery) getConfigBytes(key string) ([]byte, error) {
	d.mux.Lock()
	defer d.mux.Unlock()

	cfg, ok := d.configs[key]
	if !ok {
		return nil, errors.New("config not found")
	}

	bs, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	return bs, nil
}

var envNDStockConfigDir = os.Getenv("NETDATA_STOCK_CONFIG_DIR")

func isStock(cfg confgroup.Config) bool {
	if envNDStockConfigDir == "" {
		return false
	}
	return strings.HasPrefix(cfg.Source(), envNDStockConfigDir)
}
