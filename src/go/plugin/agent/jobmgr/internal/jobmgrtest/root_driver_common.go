package jobmgrtest

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ShippedRootDriver executes prebuilt production command roots against exact
// empty-job configurations.
type ShippedRootDriver struct {
	ConfigDir      string
	GoDPlugin      string
	IBMPlugin      string
	ScriptsDPlugin string
}

type shippedRoot struct {
	name       string
	executable string
	module     string
	configFile string
}

func (srd ShippedRootDriver) roots() [3]shippedRoot {
	return [3]shippedRoot{
		{name: "godplugin", executable: srd.GoDPlugin, module: "testrandom", configFile: "go.d/testrandom.conf"},
		{
			name:       "ibmdplugin",
			executable: srd.IBMPlugin,
			module:     "websphere_mp",
			configFile: "ibm.d/websphere_mp.conf",
		},
		{
			name:       "scriptsdplugin",
			executable: srd.ScriptsDPlugin,
			module:     "nagios",
			configFile: "scripts.d/nagios.conf",
		},
	}
}

func (srd ShippedRootDriver) validateConfigs() error {
	if !filepath.IsAbs(srd.ConfigDir) {
		return errors.New("jobmgr test: shipped-root config directory must be absolute")
	}
	info, err := os.Stat(srd.ConfigDir)
	if err != nil {
		return fmt.Errorf("jobmgr test: stat shipped-root config directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("jobmgr test: shipped-root config path is not a directory")
	}
	for _, root := range srd.roots() {
		path := filepath.Join(srd.ConfigDir, root.configFile)
		if err := validateEmptyJobsConfig(path); err != nil {
			return fmt.Errorf("jobmgr test: %s config: %w", root.name, err)
		}
	}
	return nil
}

func validateEmptyJobsConfig(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var config struct {
		Jobs []any `yaml:"jobs"`
	}
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return err
	}
	if config.Jobs == nil || len(config.Jobs) != 0 {
		return errors.New("expected exact empty jobs list")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected additional YAML document")
		}
		return err
	}
	return nil
}
